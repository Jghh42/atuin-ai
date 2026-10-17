#!/usr/bin/env python3
"""Opt-in Linux offline resource gate; missing prerequisites are fatal, never skip.

Outer coordinator verifies a DEVELOPMENT package then enters a no-egress network
namespace. Everything measured, fixture, load and sampling runs inside it. No
production instrumentation, custom upstream URL, forced GC or race RSS binary.
"""
import argparse
import base64
import concurrent.futures
import hashlib
import http.client
import json
import os
import pathlib
import platform
import resource
import select
import shutil
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import threading
import time

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parent.parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE))
from bwrap_tool import resolve
from tls_fixture import Fixture, Plan, certificates, compact

MIB = 1 << 20
NEAR = 4*MIB-1024
# The inner driver does not need Git or a checkout. Only these public, verified
# package-inventory entries are materialized; ignored/private files are never
# enumerated, copied or mounted. Update this closed set when imports change.
NAMESPACE_INPUTS = (
    'tests/resources/run.py', 'tests/resources/tls_fixture.py',
    'tests/resources/namespace_test.py', 'tests/bwrap_tool.py',
    'tests/build_support.py', 'testdata/prompts/default.golden',
    'testdata/tools/suggest_command.json', 'testdata/tools/write_file.json',
)


def digest(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def require(value, message):
    if not value:
        raise AssertionError(message)


def wait(event, timeout=10):
    require(event.wait(timeout), 'observable barrier timed out')


def source_snapshot(source_root, manifest, destination):
    """Copy ONLY the closed public input set, byte/mode-checked against package.

    The outer consumer has already checked the COMPLETE actual source/HEAD/package
    identity. This subset is not another Git/source truth claim inside the jail.
    """
    inventory = manifest['source_inventory']
    canonical = (json.dumps(inventory,sort_keys=True,indent=2)+'\n').encode()
    require(hashlib.sha256(canonical).hexdigest() == manifest['source']['inventory_sha256'],
            'package source inventory digest')
    entries = {entry['path']:entry for entry in inventory}
    require(len(entries) == len(inventory),'duplicate package source inventory entry')
    require(not destination.exists() and not destination.is_symlink(),'fresh minimal snapshot required')
    destination.mkdir(mode=0o700)
    selected = []
    for name in NAMESPACE_INPUTS:
        require(name in entries,'required namespace input missing from verified package')
        entry = entries[name]
        path = source_root/name
        for parent in path.parents:
            require(not parent.is_symlink(),'namespace source ancestor symlink')
            if parent == source_root:
                break
        fd = os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
        with os.fdopen(fd,'rb') as stream:
            info = os.fstat(stream.fileno())
            require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1,'namespace source file type')
            require(info.st_size == entry['size'],'namespace source bytes/mode mismatch')
            data = stream.read(entry['size']+1)
        mode = '0755' if info.st_mode & 0o111 else '0644'
        require(mode == entry['mode'] and len(data) == entry['size']
                and hashlib.sha256(data).hexdigest() == entry['sha256'],'namespace source bytes/mode mismatch')
        target = destination/name
        parent = destination
        for part in pathlib.Path(name).parts[:-1]:
            parent = parent/part
            parent.mkdir(mode=0o700,exist_ok=True)
        target.write_bytes(data)
        target.chmod(int(mode,8))
        selected.append(dict(entry))
    verify_snapshot(destination,selected)
    return selected


def verify_snapshot(root, entries):
    root_info = root.lstat()
    require(stat.S_ISDIR(root_info.st_mode) and stat.S_IMODE(root_info.st_mode) == 0o700
            and root_info.st_uid == os.geteuid(),'owned private snapshot root')
    expected_files = {entry['path']:entry for entry in entries}
    expected_dirs = {str(parent) for name in expected_files for parent in pathlib.Path(name).parents
                     if str(parent) != '.'}
    actual_files, actual_dirs = set(), set()
    for path in root.rglob('*'):
        info = path.lstat()
        name = path.relative_to(root).as_posix()
        require(not stat.S_ISLNK(info.st_mode),'snapshot symlink forbidden')
        if stat.S_ISDIR(info.st_mode):
            require(stat.S_IMODE(info.st_mode) == 0o700,'snapshot directory permissions')
            actual_dirs.add(name)
        else:
            require(name in expected_files and stat.S_ISREG(info.st_mode) and info.st_nlink == 1,
                    'unexpected snapshot file/type')
            entry = expected_files[name]
            require(stat.S_IMODE(info.st_mode) == int(entry['mode'],8)
                    and info.st_size == entry['size'] and digest(path) == entry['sha256'],
                    'snapshot bytes/mode changed')
            actual_files.add(name)
    require(actual_files == set(expected_files) and actual_dirs == expected_dirs,'exact minimal snapshot set')


def namespace_command(bwrap, source, binary, helper, scratch, out):
    # Shared by the actual resource run AND synthetic confidentiality regression.
    # No checkout/parent mount, /git, HOME, ignored data or private probe access.
    source = source.resolve(strict=True)
    require(source != REPO.resolve(strict=True),'never mount actual working checkout')
    for writable in (scratch,out):
        writable = writable.resolve(strict=True)
        require(not source.is_relative_to(writable) and not writable.is_relative_to(source),
                'source snapshot must have no writable mount alias')
    return [bwrap,'--die-with-parent','--new-session','--unshare-user','--unshare-pid','--unshare-net',
            '--tmpfs','/','--ro-bind','/usr','/usr','--symlink','usr/bin','/bin',
            '--symlink','usr/lib','/lib','--symlink','usr/lib64','/lib64',
            '--dir','/etc','--dir','/tmp','--dir','/sys',
            '--ro-bind',str(source),'/repo','--ro-bind',str(binary),'/binary',
            '--ro-bind',str(helper),'/helper','--bind',str(scratch),'/work',
            '--bind',str(out),'/evidence','--proc','/proc','--dev','/dev',
            '--chdir','/repo','--clearenv','--setenv','PATH','/usr/bin:/bin',
            '--setenv','PYTHONDONTWRITEBYTECODE','1']


def maximum_sample_interval_ms(samples):
    times = sorted(sample['elapsed'] for sample in samples)
    return max((1000*(b-a) for a,b in zip(times,times[1:])),default=None)


def body_for(history, near=False):
    body = compact({'messages':[{'role':'user','content':history}],
                    'config':{'capabilities':['client_v1_write_file']}})
    if near:
        # Meaningful decoded history, not whitespace/padding. Each escaped 'a'
        # costs six wire bytes but ONE decoded character in the token estimate.
        marker = b'"content":"'
        start = body.index(marker)+len(marker)
        prefix, suffix = body[:start], body[body.index(b'"}', start):]
        room = NEAR-len(prefix)-len(suffix)
        history = 'a'*(room//6)+'b'*(room%6)
        body = prefix+b'\\u0061'*(room//6)+b'b'*(room%6)+suffix
        require(len(body) == NEAR, 'near wire exact size')
        require(json.loads(body)['messages'][0]['content'] == history, 'near decoded history')
        require(600000 < len(history) < 720000, 'near meaningful/token estimate bound')
    return body, history


def near_plan(history):
    calls = []
    for i in range(32):
        arguments = compact({'file_path':'never-executed-'+str(i), 'content':'z'*((256*1024-2048) if i == 0 else 15*1024)}).decode()
        calls.append({'type':'function_call','id':'item-'+str(i),'call_id':'call-'+str(i),
                      'name':'write_file','arguments':arguments})
    plan = Plan(history, 'near', text_bytes=1, calls=calls)
    output = [{'id':'msg','type':'message','role':'assistant','content':[{'type':'output_text','text':''}]}]+calls
    event = {'type':'response.completed','response':{'id':'resp','status':'completed','output':output}}
    frame_size = len(b'event: response.completed\ndata: '+compact(event)+b'\n\n')
    plan.text = 'x'*(MIB-1024-frame_size)
    require(len(plan.text) > 200000, 'near finite text payload')
    return plan


class Process:
    """Owned spawn/group, actual time-child PID and external /proc sampling."""
    def __init__(self, executable, args, env, root, companion=False, owners=None, fault=None):
        self.root = root
        self.companion = companion
        self.samples = []
        self.sample_errors = []
        self.exited_permission_samples = 0
        self.stop_sampling = threading.Event()
        self.reaped = False
        self.rusage = None
        self.wrapper_pid = self.pid = self.sampler = None
        self._raw_fds, self._streams = set(), []
        self.initialization_failed = False
        try:
            self.log = open(root/'stderr.log', 'wb')
            self._streams.append(self.log)
            stdin_r, stdin_w = self._pipe()
            stdout_r, stdout_w = self._pipe()
            actions = [(os.POSIX_SPAWN_DUP2,stdin_r,0),(os.POSIX_SPAWN_DUP2,stdout_w,1),
                       (os.POSIX_SPAWN_DUP2,self.log.fileno(),2),
                       (os.POSIX_SPAWN_CLOSE,stdin_w),(os.POSIX_SPAWN_CLOSE,stdout_r)]
            # time's post-exec fork isolates maxRSS from Python's pre-exec memory.
            self.time_path = root/'time.txt'
            self.wrapper_pid = os.posix_spawn('/usr/bin/time',
                ['/usr/bin/time','-v','-o',str(self.time_path),str(executable),*args],env,
                file_actions=actions,setsid=True)
            # Register ownership BEFORE discovery, descriptor wrapping or sampler
            # setup can fail. Constructor rollback covers every later exception.
            if owners is not None:
                owners.append(self)
            if fault:
                fault('pid-discovery',self)
            deadline = time.monotonic()+3
            while True:
                children = pathlib.Path(f'/proc/{self.wrapper_pid}/task/{self.wrapper_pid}/children').read_text().split()
                if children:
                    require(len(children) == 1,'time wrapper must own exactly one measured server')
                    self.pid = int(children[0])
                    break
                require(time.monotonic() < deadline,'time child PID witness')
                time.sleep(.001)
            self._close_fd(stdin_r)
            self._close_fd(stdout_w)
            self.input = self._stream(stdin_w,'w',buffering=1)
            self.output = self._stream(stdout_r,'rb',buffering=0)
            self.sampler = threading.Thread(target=self.sample_loop)
            if fault:
                fault('sampler-start',self)
            self.sampler.start()
            if fault:
                fault('sampler-started',self)
        except BaseException:
            self.initialization_failed = True
            self._abort_initialization()
            raise

    def _pipe(self):
        pair = os.pipe()
        self._raw_fds.update(pair)
        return pair

    def _close_fd(self, fd):
        os.close(fd)
        self._raw_fds.remove(fd)

    def _stream(self, fd, mode, buffering):
        stream = os.fdopen(fd,mode,buffering=buffering)
        self._streams.append(stream)
        self._raw_fds.remove(fd)
        return stream

    def _close_resources(self):
        failures = []
        for stream in self._streams:
            try:
                stream.close()
            except OSError:
                failures.append('owned stream close failed')
        for fd in list(self._raw_fds):
            try:
                self._close_fd(fd)
            except OSError:
                failures.append('owned descriptor close failed')
        require(not failures,'; '.join(failures))

    def _abort_initialization(self):
        # This class is run in the dedicated PID namespace; its init reaps any
        # killed time-child orphan. Never target a discovered/unrelated group.
        try:
            self.stop_sampling.set()
            if self.wrapper_pid is not None:
                try:
                    os.killpg(self.wrapper_pid,signal.SIGKILL)
                except ProcessLookupError:
                    pass
                if self.sampler is not None and self.sampler.ident is not None:
                    self.sampler.join(2)
                os.wait4(self.wrapper_pid,0)
                self.reaped = True
                require(self.sampler is None or not self.sampler.is_alive(),'failed-init sampler did not join')
                deadline = time.monotonic()+2
                while True:
                    try:
                        os.killpg(self.wrapper_pid,0)
                    except ProcessLookupError:
                        break
                    require(time.monotonic() < deadline,'failed-init owned process group not reaped')
                    time.sleep(.005)
        finally:
            self._close_resources()

    def alive(self):
        return self.wrapper_pid is not None and not self.reaped and os.waitid(os.P_PID, self.wrapper_pid, os.WEXITED|os.WNOHANG|os.WNOWAIT) is None

    def sample(self):
        base = pathlib.Path('/proc')/str(self.pid)
        try:
            status = {}
            for line in (base/'status').read_text().splitlines():
                key, _, value = line.partition(':')
                if key in ('VmRSS','VmHWM','Threads'):
                    status[key] = int(value.split()[0])
            fds = list((base/'fd').iterdir())
            sockets = 0
            for fd in fds:
                try:
                    sockets += os.readlink(fd).startswith('socket:[')
                except FileNotFoundError:
                    pass
            if 'VmRSS' in status:
                self.samples.append({'elapsed':time.monotonic()-self.began, **status,
                                     'fds':len(fds),'sockets':sockets})
        except (FileNotFoundError, ProcessLookupError):
            pass

    def sample_loop(self):
        self.began = time.monotonic()
        while not self.stop_sampling.is_set():
            try:
                self.sample()
            except PermissionError:
                # Linux denies /proc/ZOMBIE/fd even to its parent inside this
                # user namespace. Independently observed waitid(WNOWAIT) exit
                # classifies that terminal race; a live-child denial FAILS.
                deadline = time.monotonic()+.5
                while self.alive() and time.monotonic() < deadline:
                    time.sleep(.001)
                if not self.alive():
                    self.exited_permission_samples += 1
                    return
                self.sample_errors.append('live owned proc FD sampling permission denied')
                return
            except Exception as exc:
                self.sample_errors.append('sampler failure: '+type(exc).__name__)
                return
            self.stop_sampling.wait(.02)

    def line(self):
        require(select.select([self.output], [], [], 5)[0], 'companion pipe timeout')
        line = self.output.readline(4096)
        require(line.endswith(b'\n'), 'companion bounded JSON line')
        return json.loads(line)

    def snapshot(self):
        require(self.companion, 'never force GC on shipping binary')
        self.input.write('snapshot\n')
        self.input.flush()
        result = self.line()
        self.sample()
        result.update({k:self.samples[-1][k] for k in ('fds','sockets')})
        return result

    def finish(self, sig=signal.SIGTERM, budget=7):
        if self.reaped:
            return self.rusage
        if self.alive() and sig is not None:
            os.kill(self.pid, sig)
        deadline = time.monotonic()+budget
        forced = False
        while self.alive():
            if time.monotonic() >= deadline:
                os.killpg(self.wrapper_pid, signal.SIGKILL)
                forced = True
                break
            time.sleep(.01)
        self.stop_sampling.set()
        self.sampler.join(2)
        require(not self.sampler.is_alive(), 'sampler did not join')
        _, status, usage = os.wait4(self.wrapper_pid, 0)
        self.reaped = True
        time_fields = {}
        for line in self.time_path.read_text().splitlines():
            key, separator, value = line.strip().rpartition(': ')
            if separator:
                time_fields[key] = value
        self.rusage = {'max_rss_kib':int(time_fields['Maximum resident set size (kbytes)']),
                       'user_seconds':float(time_fields['User time (seconds)']),
                       'system_seconds':float(time_fields['System time (seconds)']),
                       'exit_code':os.waitstatus_to_exitcode(status),'forced_kill':forced,
                       'accounting':'GNU time -v, actual post-exec server child',
                       'wrapper_wait4_max_rss_kib_NOT_SERVER':usage.ru_maxrss}
        self._close_resources()
        require(not self.sample_errors, '; '.join(self.sample_errors))
        require(not forced and self.rusage['exit_code'] == 0, 'owned server failed bounded clean shutdown')
        return self.rusage

    def evidence(self):
        return {'kind':'companion' if self.companion else 'shipping', 'pid':self.pid,
                'requested_sample_period_ms':20, 'sample_count':len(self.samples),
                'max_observed_sample_interval_ms':maximum_sample_interval_ms(self.samples),
                'exited_permission_samples':self.exited_permission_samples,'sample_errors':self.sample_errors,
                'peak_rss_kib':max(s['VmRSS'] for s in self.samples),
                'peak_hwm_kib':max(s['VmHWM'] for s in self.samples),
                'peak_fds':max(s['fds'] for s in self.samples),
                'peak_owned_socket_inodes':max(s['sockets'] for s in self.samples),
                'kernel_rusage':self.rusage, 'samples':self.samples,
                'wrapper_pid_NOT_MEASURED':self.wrapper_pid}


class Suite:
    def __init__(self, args, root, report):
        self.args, self.root, self.report = args, root, report
        self.serial = 0
        self.processes = []
        self.environments = {}
        ca = root/'ca'
        ca.mkdir(mode=0o700)
        (root/'empty-certs').mkdir(mode=0o700)
        certificates(ca)
        account = 'resource-synthetic-account'
        claims = compact({'exp':int(time.time())+7200,'https://api.openai.com/auth':{'chatgpt_account_id':account}})
        token = 'e30.'+base64.urlsafe_b64encode(claims).rstrip(b'=').decode()+'.c2ln'
        self.fake = {'token':token,'account':account}
        self.fake_path = root/'synthetic.json'
        self.fake_path.write_bytes(compact(self.fake))
        self.fake_path.chmod(0o600)
        self.fixture = Fixture(ca, self.fake)

    def environment(self, root):
        for name in ('home','config','state','tmp'):
            (root/name).mkdir(mode=0o700)
        return {'PATH':'/usr/bin:/bin','HOME':str(root/'home'),'XDG_CONFIG_HOME':str(root/'config'),
                'XDG_STATE_HOME':str(root/'state'),'TMPDIR':str(root/'tmp'),
                'HTTPS_PROXY':'http://127.0.0.1:'+str(self.fixture.port),
                'SSL_CERT_FILE':str(self.root/'ca'/'ca.pem'),'SSL_CERT_DIR':str(self.root/'empty-certs'),
                'GOTRACEBACK':'none'}

    def start(self, label, companion=False, empty=False, reuse=None):
        root = self.root/(label+'-'+str(len(self.processes)))
        root.mkdir(mode=0o700)
        if reuse is None:
            env = self.environment(root)
            if not empty:
                result = subprocess.run([self.args.helper, 'setup', str(self.fake_path)], env=env,
                                        capture_output=True, timeout=10)
                require(result.returncode == 0, 'synthetic Manager.Login setup failed')
        else:
            env = self.environments[reuse]
        self.environments[label] = env
        executable = self.args.helper if companion else self.args.binary
        process = Process(executable, ['serve'], env, root, companion,owners=self.processes)
        if companion:
            require(process.line() == {'companion_ready':True}, 'companion readiness')
        deadline = time.monotonic()+5
        while True:
            require(process.alive(), 'server exited before listener')
            try:
                with socket.create_connection(('127.0.0.1',17391), .2):
                    break
            except OSError:
                require(time.monotonic() < deadline, 'server listen deadline')
                time.sleep(.01)
        if not empty:
            conn = http.client.HTTPConnection('127.0.0.1',17391, timeout=10)
            conn.request('GET','/api/cli/models')
            resp = conn.getresponse()
            require(resp.status == 200 and json.loads(resp.read())['default'] == 'gpt-6.1-sol', 'native catalog warmup')
            conn.close()
        return process

    def plan(self, mode='normal', near=False, text_bytes=65536):
        self.serial += 1
        history = ('resource-'+str(self.serial)+':').ljust(16384, 'h')
        body, history = body_for(history, near)
        if near:
            body = body.replace(b'\\u0061', b'\\u0063', self.serial)
            history = json.loads(body)['messages'][0]['content']
        plan = near_plan(history) if near else Plan(history, mode, text_bytes)
        self.fixture.add(plan)
        return plan, body

    @staticmethod
    def request(body, partial=False):
        sock = socket.create_connection(('127.0.0.1',17391), 5)
        sock.settimeout(15)
        head = (f'POST /api/cli/chat HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nConnection: close\r\nContent-Length: {len(body)}\r\n\r\n').encode()
        sock.sendall(head+body[:-1] if partial else head+body)
        return sock

    @staticmethod
    def response(sock):
        resp = http.client.HTTPResponse(sock)
        resp.begin()
        require(resp.status == 200, 'admitted request must stream HTTP200')
        require(resp.getheader('Content-Type') == 'text/event-stream', 'native SSE response')
        return resp

    @staticmethod
    def read_to_text(resp, expected):
        count = 0
        while True:
            line = resp.readline(2*MIB)
            require(line, 'stream ended before text witness')
            if line.startswith(b'event: text'):
                data = resp.readline(2*MIB)
                value = json.loads(data.removeprefix(b'data: '))
                count += len(value['content'])
                if count >= expected:
                    require(count == expected, 'exact text length')
                    return
            require(not line.startswith(b'event: error'), 'provider failed before text witness')

    @staticmethod
    def terminal(resp, plan, text_already=False):
        data = resp.read(4*MIB)
        require(resp.read(1) == b'', 'bounded downstream output')
        require(data.count(b'event: done\n') == 1 and b'event: error' not in data, 'exact successful terminal')
        require(data.count(b'event: tool_call\n') == len(plan.calls), 'all bounded calls emitted once')
        if not text_already:
            require(data.count(b'event: text\n') == 1, 'normal one text frame')
        resp.close()

    def successful(self):
        plan, body = self.plan()
        plan.release.set()
        plan.finish.set()
        sock = self.request(body)
        try:
            resp = self.response(sock)
            self.terminal(resp, plan)
            wait(plan.closed)
            require(plan.generations == 1, 'one native generation per request')
        finally:
            sock.close()
        self.fixture.check()
        return plan.summary()

    def canceled(self, mode='disconnect', companion=None):
        plan, body = self.plan(mode)
        sock = self.request(body)
        try:
            resp = self.response(sock)
            wait(plan.started)
            if mode == 'disconnect':
                observed = time.monotonic()
                # Closing the owning TCP socket is the black-box witness. Do not
                # call HTTPResponse.close first: it can retain/duplicate an fd.
                sock.shutdown(socket.SHUT_RDWR)
                sock.close()
                resp.close()
                plan.release.set()
                wait(plan.closed, 1)
                elapsed = plan.closed_at-observed
                require(0 <= elapsed <= 1, 'disconnect to upstream close exceeds 1s')
            else:
                sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 1024)
                plan.release.set()
                # Black-box observes upstream EOF, NOT the exact server write
                # timeout. The companion/unit TCP listener test supplies that
                # separate error-to-cleanup witness; do not fabricate one here.
                wait(plan.closed, 12)
                elapsed = plan.closed_at-plan.headers_at
                require(4.5 <= elapsed <= 11, 'default nonreading backpressure bound')
                resp.close()
        finally:
            sock.close()
        self.fixture.check()
        result = {'mode':mode,'elapsed_seconds':elapsed,'generation_count':plan.generations,
                  'write_timeout_error_witness':False}
        if mode == 'backpressure' and companion is not None:
            companion.input.write('witness\n')
            companion.input.flush()
            stats = companion.line()
            witness = stats['write_timeout']
            require(witness is not None,'companion did not observe actual TCP timeout error')
            require(witness['written'] < witness['requested'],'companion TCP write was not partial')
            require(witness['start_ns'] < witness['deadline_ns'] <= witness['end_ns'],'companion write deadline witness')
            require(4_900_000_000 <= witness['deadline_ns']-witness['start_ns'] <= 5_000_000_000,'actual default five-second write deadline')
            cleanup_ns = plan.closed_at_ns-witness['end_ns']
            require(0 <= cleanup_ns <= 1_000_000_000,'actual timeout-error to upstream EOF exceeds 1s')
            result.update({'write_timeout_error_witness':True,'companion_only':True,
                           'witness':witness,'timeout_error_to_upstream_close_ns':cleanup_ns,
                           'clock':'same-host realtime nanoseconds; deadline itself observed by Go TCP'})
        return result

    @staticmethod
    def partial_admission_witness(process, sockets):
        peers = {sock.getsockname()[1] for sock in sockets}
        deadline = time.monotonic()+3
        while True:
            owned = set()
            for path in (pathlib.Path('/proc')/str(process.pid)/'fd').iterdir():
                try:
                    name = os.readlink(path)
                    if name.startswith('socket:['):
                        owned.add(name[8:-1])
                except FileNotFoundError:
                    pass
            consumed, acknowledged = set(), set()
            for line in pathlib.Path('/proc/net/tcp').read_text().splitlines()[1:]:
                fields = line.split()
                local, remote = int(fields[1].split(':')[1],16), int(fields[2].split(':')[1],16)
                tx, rx = (int(v,16) for v in fields[4].split(':'))
                if local == 17391 and remote in peers and fields[9] in owned and rx == 0:
                    consumed.add(remote)
                if local in peers and remote == 17391 and tx == 0:
                    acknowledged.add(local)
            if consumed == peers == acknowledged:
                return
            require(time.monotonic() < deadline, 'partial-body owned socket queues not drained')
            time.sleep(.005)

    def rss(self, near=False, idle=False):
        label = 'idle' if idle else ('near' if near else 'normal')
        process = self.start('rss-'+label)
        # Warm real types/caches with production composition, no forced GC.
        self.successful()
        if idle:
            time.sleep(2)
            idle_samples = process.samples[-50:]
            idle_rss = max(s['VmRSS'] for s in idle_samples)
            witness = {'warmed_idle_rss_kib':idle_rss, 'warmup_seconds':2}
        else:
            plans, bodies = [], []
            for i in range(4):
                plan, body = self.plan(near=near)
                plans.append(plan)
                bodies.append(body)
            sockets = [self.request(body, partial=True) for body in bodies]
            responses = []
            try:
                # Admission barrier: all four bodies lack their last byte. A
                # fifth 503 proves no hidden parse/reject/queue was substituted.
                # Kernel owned-socket receive/send queue witness proves all four
                # >bufio-size partial bodies were actually consumed, not just sent.
                self.partial_admission_witness(process, sockets)
                conn = http.client.HTTPConnection('127.0.0.1',17391,timeout=5)
                conn.request('POST','/api/cli/chat',b'{}')
                fifth = conn.getresponse()
                require(fifth.status == 503 and json.loads(fifth.read())['error'] == 'busy', 'four predecode admission slots')
                conn.close()
                for sock, body in zip(sockets,bodies):
                    sock.sendall(body[-1:])
                responses = [self.response(sock) for sock in sockets]
                for plan in plans:
                    wait(plan.started)
                require(self.fixture.active == 4, 'four actual concurrent upstream generations')
                for plan in plans:
                    plan.release.set()
                with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                    list(pool.map(lambda pair:self.read_to_text(*pair), zip(responses,[len(p.text) for p in plans])))
                require(self.fixture.active == 4, 'four parsers retained calls/text before completion')
                time.sleep(.1)  # sampling dwell AFTER observable parser witnesses
                for plan in plans:
                    plan.finish.set()
                with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                    list(pool.map(lambda pair:self.terminal(*pair,text_already=True),zip(responses,plans)))
                for plan in plans:
                    wait(plan.closed)
                    require(plan.generations == 1, 'four requests exactly four generations')
                witness = {'admitted_before_decode':4,'concurrent_upstream':4,'parser_text_witnesses':4,
                           'request_bytes':[len(b) for b in bodies], 'plans':[p.summary() for p in plans]}
            finally:
                for sock in sockets:
                    sock.close()
        process.finish()
        evidence = process.evidence()
        peak = max(evidence['peak_hwm_kib'], evidence['kernel_rusage']['max_rss_kib'])
        hard = 64*1024 if idle else 256*1024
        soft = 32*1024 if idle else (128*1024 if not near else None)
        result = {**witness,**evidence,'hard_limit_kib':hard,'hard_pass':peak <= hard,
                  'soft_limit_kib':soft,'soft_pass':None if soft is None else peak <= soft}
        self.report['shipping_rss'][label] = result
        require(result['hard_pass'], 'shipping '+label+' RSS hard gate failed')
        self.fixture.check()

    @staticmethod
    def file_inventory(env):
        result = []
        for variable in ('HOME','XDG_CONFIG_HOME','XDG_STATE_HOME','TMPDIR'):
            base = pathlib.Path(env[variable])
            for path in sorted(base.rglob('*')):
                info = path.lstat()
                result.append((variable,str(path.relative_to(base)),info.st_mode,info.st_size if path.is_file() else 0))
        return result

    def cycles(self, companion=False):
        process = self.start('retention-helper' if companion else 'cycles-shipping', companion)
        # Warm the exact success and cancellation paths before the baseline.
        for _ in range(5):
            self.successful()
            self.canceled()
        snapshots = []
        if companion:
            snapshots.append({'cycles':0,**process.snapshot()})
        else:
            process.sample()
            snapshots.append({'cycles':0,**process.samples[-1]})
        before_files = self.file_inventory(self.environments['retention-helper' if companion else 'cycles-shipping'])
        cancel_results = []
        for i in range(1,101):
            self.successful()
            cancel_results.append(self.canceled())
            if i % 10 == 0:
                require(self.fixture.active == 0, 'cycle quiescence upstream count')
                if companion:
                    snapshot = process.snapshot()
                else:
                    process.sample()
                    snapshot = process.samples[-1]
                snapshots.append({'cycles':i,**snapshot})
        backpressure = self.canceled('backpressure',process if companion else None)
        after_files = self.file_inventory(self.environments['retention-helper' if companion else 'cycles-shipping'])
        require(before_files == after_files,'unexpected transcript/state file retention')
        process.finish()
        keys = ('goroutines','heap_alloc','heap_objects','fds','sockets') if companion else ('fds','sockets','Threads')
        checks = retention_checks(snapshots, keys)
        result = {'completed_cycles':100,'disconnect_cycles':100,'warmup_completed':5,'warmup_canceled':5,
                  'cancellations':cancel_results,'backpressure':backpressure,'snapshots':snapshots,
                  'retention':checks,'private_file_metadata_unchanged':True,
                  'private_file_count':len(after_files),**process.evidence()}
        self.report['companion_retention' if companion else 'shipping_cycles'] = result
        require(all(c['pass'] for c in checks.values()), 'sustained resource retention growth')
        self.fixture.check()

    def over_limits(self):
        process = self.start('over-limits')
        outcomes = []
        body,_ = body_for('unused',True)
        body = body.replace(b'"content":"',b'"content":"'+b'a'*1025,1)
        require(len(body) == 4*MIB+1,'exact oversized body')
        before = self.fixture.generations
        sock = self.request(body)
        try:
            resp = http.client.HTTPResponse(sock)
            resp.begin()
            require(resp.status == 413 and json.loads(resp.read())['error'] == 'request_too_large','body hard rejection')
            require(self.fixture.generations == before,'oversized body opened upstream')
            resp.close()
        finally:
            sock.close()
        outcomes.append({'mode':'over-body','wire_bytes':len(body),'status':413,'generations':0})
        for mode in ('over-event','over-args','over-calls'):
            plan,body = self.plan(mode)
            plan.release.set()
            sock = self.request(body)
            try:
                resp = self.response(sock)
                data = resp.read(4096)
                require(data.count(b'event: error\n') == 1 and b'event: done' not in data and b'event: tool_call' not in data,'provider hard rejection '+mode)
                wait(plan.closed,1)
                require(plan.generations == 1,'overlimit generation retry')
                resp.close()
                outcomes.append({'mode':mode,'generations':1,'terminal_error':1,'executable_calls':0})
            finally:
                sock.close()
        process.finish()
        self.report['over_limits'] = outcomes
        self.fixture.check()

    def timeouts(self):
        process = self.start('default-timeouts')
        outcomes = []
        for mode, lower, upper in [('idle-timeout',59,63),('generation-timeout',299,303)]:
            plan, body = self.plan(mode)
            sock = self.request(body)
            sock.settimeout(310)
            try:
                resp = self.response(sock)
                wait(plan.started)
                data = resp.read(4096)
                elapsed = time.monotonic()-plan.headers_at
                require(data.count(b'event: error\n') == 1 and b'event: done' not in data, 'default timeout terminal error exactly once')
                wait(plan.closed,1)
                require(lower <= elapsed <= upper, 'actual default duration gate '+mode)
                require(plan.generations == 1, 'no timeout retry')
                outcomes.append({'mode':mode,'elapsed_seconds':elapsed,'generation_count':1,
                                 'headers_open_body_idle':mode == 'idle-timeout',
                                 'heartbeat_active_until_deadline':mode == 'generation-timeout'})
                resp.close()
            finally:
                sock.close()
        process.finish()
        self.report['default_timeouts'] = outcomes
        self.fixture.check()

    def signals(self):
        results = []
        for sig in (signal.SIGINT,signal.SIGTERM):
            for mode in ('idle','idle-empty','partial-empty','active-four'):
                label = 'signal-'+str(sig)+'-'+mode
                process = self.start(label,empty='empty' in mode)
                socks, plans = [], []
                try:
                    if mode == 'partial-empty':
                        partial_body,_ = body_for('p'*16384)
                        socks = [self.request(partial_body,partial=True)]
                        self.partial_admission_witness(process,socks)
                    elif mode == 'active-four':
                        for _ in range(4):
                            plan, body = self.plan('disconnect')
                            plan.release.set()
                            sock = self.request(body)
                            self.response(sock)
                            wait(plan.started)
                            socks.append(sock)
                            plans.append(plan)
                        require(self.fixture.active == 4, 'signal has four live upstreams')
                    began = time.monotonic()
                    os.kill(process.pid,sig)
                    # Observable refusal, not merely process exit.
                    deadline = began+1
                    while True:
                        try:
                            with socket.create_connection(('127.0.0.1',17391),.1):
                                pass
                        except OSError:
                            break
                        require(time.monotonic() < deadline,'signal listener admission did not close')
                        time.sleep(.01)
                    process.finish(None,7)
                    elapsed = time.monotonic()-began
                    require(elapsed <= 6,'signal bounded drain/cancel')
                    for plan in plans:
                        wait(plan.closed,1)
                    restart = self.start(label+'-restart',empty='empty' in mode,reuse=label)
                    restart.finish()
                    results.append({'signal':signal.Signals(sig).name,'mode':mode,'active_generations':len(plans),
                                    'elapsed_seconds':elapsed,'listener_closed':True,'restart_reacquired_store':True})
                finally:
                    for sock in socks:
                        sock.close()
        self.report['signals'] = results
        self.fixture.check()

    def close(self):
        failures = []
        for process in self.processes:
            if not process.reaped:
                try:
                    process.finish()
                except Exception as e:
                    failures.append(str(e))
        try:
            self.fixture.close()
        except Exception as e:
            failures.append(str(e))
        self.report['fixture'] = {'catalogs':self.fixture.catalogs,'connects':self.fixture.connects,
                                  'generations':self.fixture.generations,
                                  'peak_concurrent_generations':self.fixture.peak_active,
                                  'active_after_cleanup':self.fixture.active,
                                  'handlers_joined':not any(t.is_alive() for t in self.fixture.threads)}
        self.report['cleanup'] = {'owned_processes':len(self.processes),
            'all_wrappers_reaped':all(p.reaped for p in self.processes),
            'all_server_pids_gone':all(not pathlib.Path('/proc',str(p.pid)).exists() for p in self.processes),
            'all_samplers_joined':all(p.sampler is None or not p.sampler.is_alive() for p in self.processes)}
        require(all(self.report['cleanup'][key] for key in ('all_wrappers_reaped','all_server_pids_gone','all_samplers_joined')),'owned process/sampler cleanup incomplete')
        require(not failures,'; '.join(failures))


def retention_checks(snapshots, keys):
    """Reject sustained positive slopes, not just a large absolute RSS ceiling.

    Baseline warmed first; all eleven snapshots are quiescent. Fixed small
    allowances handle Go scheduling/span/cache granularity. Never fit away a leak.
    Fail both end-to-baseline growth AND monotone tail growth above allowance.
    """
    allowances = {'goroutines':2,'metric_goroutines':2,'heap_alloc':512*1024,
                  'heap_objects':512,'fds':0,'sockets':0,'Threads':4}
    result = {}
    for key in keys:
        values = [s[key] for s in snapshots]
        allowance = allowances[key]
        tail = values[-6:]
        monotone = all(b >= a for a,b in zip(tail,tail[1:])) and tail[-1] > tail[0]
        growth = values[-1]-values[0]
        result[key] = {'values':values,'growth':growth,'allowance':allowance,
                       'monotone_tail':monotone,
                       'pass':growth <= allowance and not (monotone and tail[-1]-tail[0] > allowance)}
    return result


def namespace_evidence(host_inode):
    inode = os.readlink('/proc/self/ns/net')
    require(inode != host_inode, 'independent network namespace required')
    # /sys can describe the host namespace through the read-only bind; use the
    # kernel socket ioctl-independent proc table for namespace interface truth.
    devices = [line.split(':')[0].strip() for line in pathlib.Path('/proc/net/dev').read_text().splitlines()[2:]]
    require(devices == ['lo'], 'namespace must contain loopback ONLY')
    route = pathlib.Path('/proc/net/route').read_text().splitlines()
    require(len(route) == 1, 'namespace default/remote routes forbidden')
    with socket.socket() as sock:
        sock.settimeout(.2)
        error = sock.connect_ex(('192.0.2.1',443))
        require(error != 0, 'namespace remote-connect negative witness')
    with socket.socket() as listener:
        listener.bind(('127.0.0.1',0))
        listener.listen()
        with socket.create_connection(listener.getsockname(),1):
            peer,_ = listener.accept()
            peer.close()
    return {'distinct_from_host':True,'interfaces':devices,'route_rows':0,
            'remote_connect_errno':error,'usable_loopback':True,'namespace':inode}


def inside(args):
    os.umask(0o077)
    report = {'schema':1,'status':'RUNNING','failures':[], 'shipping_rss':{},
              'binary_sha256':digest(args.binary),'helper_sha256':digest(args.helper),
              'environment':{'kernel':platform.release(),'machine':platform.machine(),
                'cpu_count':os.cpu_count(),'page_size':os.sysconf('SC_PAGE_SIZE'),
                'cpu_model':next((l.split(':',1)[1].strip() for l in pathlib.Path('/proc/cpuinfo').read_text().splitlines() if l.startswith('model name')), 'unknown'),
                'affinity_cpus':len(os.sched_getaffinity(0)), 'gomemlimit':'UNSET','gogc':'UNSET',
                'race':False,'shipping_forced_gc':False,'go':args.go,
                'process_env_keys':['PATH','HOME','XDG_CONFIG_HOME','XDG_STATE_HOME','TMPDIR','HTTPS_PROXY','SSL_CERT_FILE','SSL_CERT_DIR','GOTRACEBACK']}}
    report['environment'].update(json.loads(pathlib.Path(args.host_limits).read_text()))
    report['environment']['rlimits'] = {name:list(resource.getrlimit(getattr(resource,name)))
        for name in ('RLIMIT_AS','RLIMIT_NOFILE','RLIMIT_NPROC')}
    suite = None
    try:
        report['namespace'] = namespace_evidence(args.host_netns)
        namespace_inputs = json.loads(pathlib.Path(args.namespace_inputs).read_text())
        require({entry['path'] for entry in namespace_inputs['files']} == set(NAMESPACE_INPUTS),
                'closed namespace input inventory')
        verify_snapshot(REPO,namespace_inputs['files'])
        require(not (REPO/'.git').exists(),'Git must not be mounted inside resource namespace')
        report['namespace_inputs'] = namespace_inputs
        suite = Suite(args,pathlib.Path(args.scratch),report)
        cases = [('rss-idle',lambda:suite.rss(idle=True)),('rss-normal',suite.rss),
                 ('rss-near',lambda:suite.rss(near=True)),('shipping-cycles',suite.cycles),
                 ('companion-retention',lambda:suite.cycles(True)),('over-limits',suite.over_limits),('signals',suite.signals),
                 ('default-timeouts',suite.timeouts)]
        for name,call in cases:
            if args.case and name not in args.case:
                continue
            began = time.monotonic()
            try:
                call()
            except Exception as e:
                report['failures'].append({'case':name,'error':str(e),'type':type(e).__name__})
                # Preserve the FIRST failure, don't retry/minimize/rebudget.
                # Independent cases continue only once owned process is stopped.
                for process in suite.processes:
                    if not process.reaped:
                        try:
                            process.finish()
                        except Exception as cleanup:
                            report['failures'].append({'case':name+'-cleanup','error':str(cleanup)})
                print('FAIL '+name+': '+str(e),flush=True)
            else:
                print('PASS '+name,flush=True)
            report.setdefault('elapsed_seconds',{})[name] = time.monotonic()-began
            pathlib.Path(args.report).write_text(json.dumps(report,indent=2)+'\n')
    except Exception as e:
        report['failures'].append({'case':'prerequisite','error':str(e),'type':type(e).__name__})
    finally:
        if suite:
            try:
                suite.close()
            except Exception as e:
                report['failures'].append({'case':'cleanup','error':str(e)})
        if suite and not args.case and not report['failures']:
            if suite.fixture.generations != 446 or suite.fixture.catalogs != 0:
                report['failures'].append({'case':'total-counts','error':'full suite must have exactly 446 generations and zero automatic remote catalogs (bundled runtime metadata)'})
        report['status'] = 'FAIL' if report['failures'] else ('PARTIAL' if args.case else 'PASS')
        pathlib.Path(args.report).write_text(json.dumps(report,indent=2)+'\n')
    return int(bool(report['failures']))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifacts', type=pathlib.Path)
    parser.add_argument('--out', type=pathlib.Path, required=True)
    parser.add_argument('--case', action='append', choices=['rss-idle','rss-normal','rss-near','shipping-cycles','companion-retention','over-limits','signals','default-timeouts'])
    parser.add_argument('--inside',action='store_true',help=argparse.SUPPRESS)
    for name in ('binary','helper','scratch','report','host-netns','host-limits','namespace-inputs','go'):
        parser.add_argument('--'+name,help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.inside:
        return inside(args)
    require(platform.system() == 'Linux' and platform.machine() == 'x86_64', 'BLOCKED: Linux amd64 required')
    require(args.artifacts is not None, 'BLOCKED: --artifacts DEVELOPMENT package required')
    artifacts = args.artifacts.resolve(strict=True)
    out = args.out.absolute()
    require(not out.exists() and not out.is_symlink(),'fresh output required')
    out.mkdir(mode=0o700,parents=True)
    os.umask(0o077)
    # Independent consumer, not self-described producer inventories. Mutations
    # are separately required by packaging CI; here verify the real exact assets.
    sys.path.insert(0,str(HERE.parent))
    import packaging_contract
    manifest = packaging_contract.verify(artifacts,REPO)
    require(manifest['mode'] == 'DEVELOPMENT; RELEASE BLOCKED','resource evidence requires an honest DEVELOPMENT package')
    binaries = list(artifacts.glob('atuin-ai-server-*-linux-amd64'))
    require(len(binaries) == 1,'exact shipping Linux asset')
    binary = binaries[0]
    require(shutil.which('openssl') and pathlib.Path('/usr/bin/time').is_file(),'BLOCKED: openssl and GNU /usr/bin/time required')
    toolchain = 'go'+next(line.split()[1] for line in (REPO/'go.mod').read_text().splitlines() if line.startswith('go '))
    build_env = {k:v for k,v in os.environ.items() if k in ('PATH','HOME','GOPATH','GOMODCACHE')}
    build_env.update({'GOTOOLCHAIN':toolchain,'GOENV':'off','GOWORK':'off','CGO_ENABLED':'0'})
    helper = out/'resourcehelper'
    subprocess.run(['go','build','-trimpath','-buildvcs=false','-mod=readonly','-o',str(helper),'./tests/resourcehelper'],
                   cwd=REPO,env=build_env,check=True,timeout=120)
    bwrap = resolve()
    version = subprocess.run([bwrap,'--version'],check=True,capture_output=True,text=True,timeout=5).stdout.strip()
    require(version == 'bubblewrap 0.11.1','BLOCKED: verified Bubblewrap 0.11.1 required')
    return run_confined(args,bwrap,binary,helper,out,manifest,artifacts,version,toolchain)


def run_confined(args,bwrap,binary,helper,out,manifest,artifacts,version,toolchain):
    """Both private trees are cleaned even if snapshot creation/launch fails."""
    with tempfile.TemporaryDirectory(prefix='phase9b-inputs-') as source_parent, \
            tempfile.TemporaryDirectory(prefix='phase9b-work-') as scratch_name:
        source = pathlib.Path(source_parent)/'inputs'
        selected = source_snapshot(REPO,manifest,source)
        scratch = pathlib.Path(scratch_name)
        result,receipt = run_confined_snapshot(args,bwrap,binary,helper,out,manifest,artifacts,
                                              version,toolchain,source,selected,scratch)
        verify_snapshot(source,selected)
    receipt.update({'scratch_removed':not scratch.exists(),
                    'source_snapshot_removed':not pathlib.Path(source_parent).exists()})
    (out/'provenance.json').write_text(json.dumps(receipt,indent=2)+'\n')
    return result


def run_confined_snapshot(args,bwrap,binary,helper,out,manifest,artifacts,version,toolchain,source,selected,scratch):
    report = out/'report.json'
    selected_digest = hashlib.sha256((json.dumps(selected,sort_keys=True,indent=2)+'\n').encode()).hexdigest()
    inputs = {'package_source_inventory_sha256':manifest['source']['inventory_sha256'],
              'selected_inventory_sha256':selected_digest,
              'files':selected,'read_only_mount':True,'whole_checkout_mounted':False}
    (out/'namespace-inputs.json').write_text(json.dumps(inputs,indent=2)+'\n')
    own_cgroup = next((line[3:] for line in pathlib.Path('/proc/self/cgroup').read_text().splitlines() if line.startswith('0::')),None)
    limits = []
    if own_cgroup is not None:
        base = pathlib.Path('/sys/fs/cgroup')
        current = base/own_cgroup.lstrip('/')
        while current.is_relative_to(base):
            row = {}
            for name in ('memory.max','memory.high','cpu.max','pids.max'):
                path = current/name
                row[name] = path.read_text().strip() if path.exists() else 'unavailable'
            limits.append(row)
            if current == base:
                break
            current = current.parent
    (out/'host-limits.json').write_text(json.dumps({'own_cgroup_and_ancestor_limits':limits,
        'physical_memory_bytes':os.sysconf('SC_PHYS_PAGES')*os.sysconf('SC_PAGE_SIZE')})+'\n')
    command = namespace_command(bwrap,source,binary,helper,scratch,out)+[
               sys.executable,'-B','-I','/repo/tests/resources/run.py','--inside','--out','/evidence',
               '--binary','/binary','--helper','/helper','--scratch','/work',
               '--report','/evidence/report.json','--host-netns',os.readlink('/proc/self/ns/net'),
               '--host-limits','/evidence/host-limits.json',
               '--namespace-inputs','/evidence/namespace-inputs.json','--go',toolchain]
    for case in args.case or []:
        command += ['--case',case]
    receipt = {'bwrap':version,'bwrap_sha256':digest(bwrap),'binary_sha256':digest(binary),
               'package_manifest_sha256':digest(artifacts/'manifest.json'),
               'artifact_path':str(binary),'helper_sha256':digest(helper),
               'source':manifest['source'],'namespace_inputs':inputs,
               'package_environment':manifest['environment'],'package_toolchain':manifest['toolchain'],
               'package_target_build':[{k:v for k,v in t.items() if k not in ('archive_inventory','legal_inventory')} for t in manifest['targets']],
               'trust':'local same-host consistency, not independent producer attestation'}
    try:
        result = subprocess.run(command, timeout=800)
        if not report.exists():
            report.write_text(json.dumps({'status':'BLOCKED','failures':['namespace/driver did not produce evidence']})+'\n')
        return result.returncode,receipt
    except subprocess.TimeoutExpired:
        unfinished = json.loads(report.read_text()) if report.exists() else {'failures':[]}
        unfinished['status'] = 'FAIL'
        unfinished.setdefault('failures',[]).append({'case':'outer-wall-guard','error':'800-second suite guard expired; no retry'})
        report.write_text(json.dumps(unfinished,indent=2)+'\n')
        return 124,receipt


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as exc:
        print('BLOCKED/FAIL: '+str(exc),file=sys.stderr)
        sys.exit(1)
