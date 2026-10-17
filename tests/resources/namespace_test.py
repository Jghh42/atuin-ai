#!/usr/bin/env python3
"""Explicit fast namespace/confidentiality/ownership/TLS-wire gate, NOT RSS proof.

The outer test creates its OWN synthetic checkout and ignored/private sentinels.
It never places or reads sentinels in the real checkout's .git/private material.
All runtime wire/failure-injection tests use the SAME narrow namespace builder
as the resource suite, with public inventory inputs and /usr/bin/true stand-ins.
Missing namespaces/prerequisites FAIL, never skip or use the host network.
"""
import hashlib
import http.client
import json
import os
import pathlib
import socket
import ssl
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
from run import (NAMESPACE_INPUTS, REPO, Process, namespace_command,
                 namespace_evidence, require, source_snapshot, verify_snapshot)
from tls_fixture import Fixture, MODELS, certificates

SENTINELS = ('.git/phase9b-synthetic-sentinel', 'private-probe/credentials.json',
             'dist/retained-probe/config.toml', 'secrets/auth.json',
             '.env.synthetic', 'config.local.toml')


def setUpModule():
    config = json.loads(pathlib.Path('/evidence/check-inputs.json').read_text())
    namespace_evidence(config['host_netns'])


class Confidentiality(unittest.TestCase):
    def test_only_verified_public_inputs_available(self):
        data = json.loads(pathlib.Path('/evidence/check-inputs.json').read_text())
        verify_snapshot(pathlib.Path('/repo'),data['files'])
        self.assertEqual({entry['path'] for entry in data['files']},set(NAMESPACE_INPUTS))
        for name in SENTINELS:
            with self.subTest(name=name):
                path = pathlib.Path('/repo')/name
                self.assertFalse(path.exists())
                with self.assertRaises(FileNotFoundError):
                    path.open('rb')
        self.assertFalse(pathlib.Path(data['synthetic_host_root']).exists())
        self.assertFalse(pathlib.Path('/repo/.git').exists())
        self.assertFalse(pathlib.Path('/repo/private-probe').exists())
        self.assertTrue(pathlib.Path('/repo/testdata/prompts/default.golden').is_file())
        for name in ('/binary','/helper'):
            self.assertTrue(pathlib.Path(name).is_file())
            subprocess.run([name],check=True,timeout=2)  # verified true stand-ins

    def test_source_mount_is_read_only(self):
        # No write/truncation occurs even if a regression makes the open succeed.
        try:
            fd = os.open('/repo/tests/resources/run.py',os.O_WRONLY)
        except OSError as exc:
            self.assertEqual(exc.errno,30)  # EROFS, not merely owner permissions
        else:
            os.close(fd)
            self.fail('verified source mount is writable')


class InitializationOwnership(unittest.TestCase):
    def test_successful_initialization_samples_actual_child_and_closes(self):
        with tempfile.TemporaryDirectory(dir='/work') as directory:
            before = set(pathlib.Path('/proc/self/fd').iterdir())
            owners = []
            process = Process('/usr/bin/python3',['-I','-c','import sys; sys.stdin.buffer.read(1)'],
                              {'PATH':'/usr/bin:/bin'},pathlib.Path(directory),owners=owners)
            try:
                self.assertEqual(owners,[process])
                self.assertNotEqual(process.pid,process.wrapper_pid)
                deadline = time.monotonic()+2
                while len(process.samples) < 2:
                    self.assertTrue(process.alive())
                    self.assertLess(time.monotonic(),deadline,'sampling witness deadline')
                    time.sleep(.005)
                process.input.write('x')
                process.input.flush()
                process.finish(None,budget=2)
                evidence = process.evidence()
                self.assertEqual(evidence['requested_sample_period_ms'],20)
                self.assertGreater(evidence['max_observed_sample_interval_ms'],0)
                self.assertEqual(evidence['kernel_rusage']['exit_code'],0)
            finally:
                if not process.reaped:
                    process._abort_initialization()
            self.assertTrue(all(stream.closed for stream in process._streams))
            self.assertFalse(process._raw_fds)
            self.assertFalse(process.sampler.is_alive())
            self.assertEqual(set(pathlib.Path('/proc/self/fd').iterdir()),before)

    def test_failure_after_spawn_discovery_and_sampler_cleans_everything(self):
        unrelated = subprocess.Popen(['/usr/bin/sleep','60'],start_new_session=True)
        try:
            for stage in ('pid-discovery','sampler-start','sampler-started'):
                with self.subTest(stage=stage), tempfile.TemporaryDirectory(dir='/work') as directory:
                    before_fds = set(pathlib.Path('/proc/self/fd').iterdir())
                    owners = []
                    def fail(point,process):
                        self.assertIn(process,owners)  # ownership precedes failure
                        if point == stage:
                            raise OSError('injected initialization failure: '+stage)
                    with self.assertRaisesRegex(OSError,'injected initialization failure'):
                        Process('/usr/bin/sleep',['60'],{'PATH':'/usr/bin:/bin'},
                                pathlib.Path(directory),owners=owners,fault=fail)
                    self.assertEqual(len(owners),1)
                    owned = owners[0]
                    self.assertTrue(owned.initialization_failed)
                    self.assertTrue(owned.reaped)
                    self.assertFalse(pathlib.Path('/proc',str(owned.wrapper_pid)).exists())
                    with self.assertRaises(ProcessLookupError):
                        os.killpg(owned.wrapper_pid,0)
                    self.assertTrue(owned.sampler is None or not owned.sampler.is_alive())
                    self.assertFalse(owned._raw_fds)
                    self.assertTrue(all(stream.closed for stream in owned._streams))
                    self.assertEqual(set(pathlib.Path('/proc/self/fd').iterdir()),before_fds)
                    self.assertIsNone(unrelated.poll())
        finally:
            unrelated.terminate()
            unrelated.wait(2)

    def test_actual_sampler_start_exception_rolls_back_registered_child(self):
        with tempfile.TemporaryDirectory(dir='/work') as directory:
            before = set(pathlib.Path('/proc/self/fd').iterdir())
            owners = []
            with mock.patch('threading.Thread.start',side_effect=OSError('injected Thread.start')), \
                    self.assertRaisesRegex(OSError,'injected Thread.start'):
                Process('/usr/bin/sleep',['60'],{'PATH':'/usr/bin:/bin'},
                        pathlib.Path(directory),owners=owners)
            self.assertEqual(len(owners),1)
            self.assertTrue(owners[0].reaped)
            with self.assertRaises(ProcessLookupError):
                os.killpg(owners[0].wrapper_pid,0)
            self.assertFalse(owners[0].sampler.is_alive())
            self.assertEqual(set(pathlib.Path('/proc/self/fd').iterdir()),before)

    def test_spawn_and_second_pipe_failure_also_close_descriptors(self):
        for failure in ('spawn','second-pipe'):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory(dir='/work') as directory:
                before = set(pathlib.Path('/proc/self/fd').iterdir())
                owners = []
                original_pipe = os.pipe
                calls = 0
                def pipe():
                    nonlocal calls
                    calls += 1
                    if calls == 2:
                        raise OSError('injected second pipe')
                    return original_pipe()
                target = 'os.posix_spawn' if failure == 'spawn' else 'os.pipe'
                kwargs = {'side_effect':OSError('injected spawn')} if failure == 'spawn' else {'side_effect':pipe}
                with mock.patch(target,**kwargs), self.assertRaisesRegex(OSError,'injected'):
                    Process('/usr/bin/sleep',['60'],{'PATH':'/usr/bin:/bin'},
                            pathlib.Path(directory),owners=owners)
                self.assertEqual(owners,[])
                self.assertEqual(set(pathlib.Path('/proc/self/fd').iterdir()),before)


class TLSWire(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root = pathlib.Path('/work/wire-ca')
        cls.root.mkdir(mode=0o700)
        certificates(cls.root)

    def run_wire(self, action, expected=()):
        fixture = Fixture(self.root,{'token':'wire-synthetic-token','account':'wire-synthetic-account'})
        attempted_egress = []
        original_connect = socket.socket.connect
        def connect(sock,address):
            if address != ('127.0.0.1',fixture.port):
                attempted_egress.append('unexpected connect')
                raise AssertionError('wire test forbids non-fixture dial')
            return original_connect(sock,address)
        def resolve(*args,**kwargs):
            attempted_egress.append('DNS resolution')
            raise AssertionError('wire test forbids all DNS resolution')
        with mock.patch.object(socket.socket,'connect',new=connect), \
                mock.patch.object(socket,'getaddrinfo',side_effect=resolve), \
                mock.patch.object(socket,'create_connection',side_effect=AssertionError('no dial helper')):
            try:
                action(fixture)
            finally:
                if expected:
                    with self.assertRaises(AssertionError):
                        fixture.close()  # boundedly joins BEFORE checking errors
                    for message in expected:
                        self.assertTrue(any(error.startswith(message) for error in fixture.errors),fixture.errors)
                    self.assertTrue(all(any(error.startswith(message) for message in expected)
                                        for error in fixture.errors),fixture.errors)
                else:
                    fixture.close()
        self.assertFalse(attempted_egress)
        self.assertFalse(any(thread.is_alive() for thread in fixture.threads))
        self.assertFalse(fixture.sockets)
        self.assertEqual(fixture.active,0)
        return fixture

    @staticmethod
    def connect(fixture, authority='chatgpt.com:443', extra=b''):
        raw = socket.socket()
        raw.settimeout(2)
        raw.connect(('127.0.0.1',fixture.port))
        raw.sendall(('CONNECT '+authority+' HTTP/1.1\r\nHost: '+authority+
                     '\r\nUser-Agent: Go-http-client/1.1\r\n').encode()+extra+b'\r\n')
        return raw

    def tunnel(self, fixture, server_name='chatgpt.com'):
        raw = self.connect(fixture)
        line,headers = Fixture.headers(raw)
        self.assertEqual(line,'HTTP/1.1 200 Connection Established')
        self.assertEqual(headers,{})
        context = ssl.create_default_context(cafile=str(self.root/'ca.pem'))
        if server_name is None:
            context.check_hostname = False  # negative missing-SNI client only
        try:
            return context.wrap_socket(raw,server_hostname=server_name)
        except BaseException:
            raw.close()
            raise

    @staticmethod
    def catalog(sock, host='chatgpt.com', extra=b''):
        sock.sendall(('GET '+MODELS+' HTTP/1.1\r\nHost: '+host+
                      '\r\nAuthorization: Bearer wire-synthetic-token'
                      '\r\nChatGPT-Account-Id: wire-synthetic-account'
                      '\r\nUser-Agent: atuin-ai/phase3a\r\nAccept: application/json\r\n').encode()+extra+b'\r\n')

    def test_actual_positive_native_catalog_wire(self):
        def action(fixture):
            sock = self.tunnel(fixture)
            try:
                self.catalog(sock)
                response = http.client.HTTPResponse(sock)
                response.begin()
                self.assertEqual(response.status,200)
                self.assertEqual(json.loads(response.read())['models'][0]['slug'],'gpt-6.1-sol')
                response.close()
            finally:
                sock.close()
        fixture = self.run_wire(action)
        self.assertEqual(fixture.connects,1)
        self.assertEqual(fixture.catalogs,1)

    def test_wrong_connect_authority_and_duplicate_host_are_denied_on_wire(self):
        for authority,extra,expected in [('else.invalid:443',b'','fixture CONNECT contract'),
                                         ('chatgpt.com:80',b'','fixture CONNECT contract'),
                                         ('chatgpt.com:443',b'Host: chatgpt.com:443\r\n','fixture duplicate header')]:
            with self.subTest(authority=authority,extra=bool(extra)):
                def action(fixture):
                    with self.connect(fixture,authority,extra) as sock:
                        self.assertEqual(sock.recv(1),b'')
                fixture = self.run_wire(action,[expected])
                self.assertEqual(fixture.connects,0)
                self.assertEqual(fixture.catalogs,0)

    def test_wrong_or_missing_sni_denied_on_wire(self):
        for name in ('wrong.invalid',None):
            with self.subTest(sni=name):
                def action(fixture):
                    with self.assertRaises(ssl.SSLError):
                        self.tunnel(fixture,name)
                fixture = self.run_wire(action,['fixture SNI mismatch','fixture TLS error:'])
                self.assertEqual(fixture.catalogs,0)

    def test_inner_host_and_framing_denied_on_wire(self):
        for host,extra,expected in [('wrong.invalid',b'','fixture destination'),
                                    ('chatgpt.com',b'Content-Length: -1\r\n','fixture body bound'),
                                    ('chatgpt.com',b'Transfer-Encoding: chunked\r\n','fixture body bound'),
                                    ('chatgpt.com',b'Content-Length: 0\r\nContent-Length: 0\r\n','fixture duplicate header')]:
            with self.subTest(host=host,framing=extra):
                def action(fixture):
                    with self.tunnel(fixture) as sock:
                        self.catalog(sock,host,extra)
                        self.assertEqual(sock.recv(1),b'')
                fixture = self.run_wire(action,[expected])
                self.assertEqual(fixture.catalogs,0)
                self.assertEqual(fixture.generations,0)


def launch():
    os.umask(0o077)
    # Synthetic source is assembled from public required paths ONLY. Nothing in
    # the real .git or ignored/private directories is accessed by this test.
    with tempfile.TemporaryDirectory(prefix='phase9b-isolation-test-') as temporary:
        root = pathlib.Path(temporary)
        host = root/'synthetic-checkout'
        host.mkdir(mode=0o700)
        inventory = []
        for name in NAMESPACE_INPUTS:
            origin = REPO/name
            data = origin.read_bytes()
            mode = '0755' if origin.stat().st_mode & 0o111 else '0644'
            target = host/name
            target.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
            target.write_bytes(data)
            target.chmod(int(mode,8))
            inventory.append({'path':name,'mode':mode,'size':len(data),'sha256':hashlib.sha256(data).hexdigest()})
        inventory.sort(key=lambda entry:entry['path'])
        canonical = (json.dumps(inventory,sort_keys=True,indent=2)+'\n').encode()
        manifest = {'source_inventory':inventory,'source':{'inventory_sha256':hashlib.sha256(canonical).hexdigest()}}
        env = {'PATH':'/usr/bin:/bin','HOME':str(root),'GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':'/dev/null'}
        subprocess.run(['git','init','-q',str(host)],env=env,check=True,capture_output=True,timeout=5)
        (host/'.gitignore').write_text('private-probe/\ndist/\nsecrets/\n.env*\nconfig.local.toml\n')
        for name in SENTINELS:
            path = host/name
            path.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
            path.write_text('SYNTHETIC CONFIDENTIALITY SENTINEL ONLY\n')
            path.chmod(0o600)
        subprocess.run(['git','-C',str(host),'check-ignore','-q','private-probe/credentials.json'],
                       env=env,check=True,timeout=5)
        snapshot = root/'snapshot'
        selected = source_snapshot(host,manifest,snapshot)
        work,out = root/'work',root/'evidence'
        work.mkdir(mode=0o700)
        out.mkdir(mode=0o700)
        (out/'check-inputs.json').write_text(json.dumps({'files':selected,
            'synthetic_host_root':str(host),'host_netns':os.readlink('/proc/self/ns/net')})+'\n')
        sys.path.insert(0,str(HERE.parent))
        from bwrap_tool import resolve
        bwrap = resolve()
        version = subprocess.run([bwrap,'--version'],check=True,capture_output=True,text=True,timeout=5)
        require(version.stdout.strip() == 'bubblewrap 0.11.1','verified Bubblewrap 0.11.1 required')
        command = namespace_command(bwrap,snapshot,pathlib.Path('/usr/bin/true'),
                                    pathlib.Path('/usr/bin/true'),work,out)
        command += [sys.executable,'-B','-I','/repo/tests/resources/namespace_test.py','--inside']
        result = subprocess.run(command,timeout=45)
        verify_snapshot(snapshot,selected)
        require(all((host/name).is_file() for name in SENTINELS),'synthetic host sentinels were altered')
        return result.returncode


if __name__ == '__main__':
    if '--inside' in sys.argv:
        unittest.main(argv=[sys.argv[0]],verbosity=2)
    else:
        try:
            sys.exit(launch())
        except Exception as exc:
            print('BLOCKED/FAIL namespace regressions: '+str(exc),file=sys.stderr)
            sys.exit(1)
