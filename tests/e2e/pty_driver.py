#!/usr/bin/env python3
"""Synthetic Atuin PTY driver. MUST run inside confinement.py's namespace."""
import errno
import fcntl
import json
import os
import pathlib
import pty
import re
import select
import signal
import stat
import struct
import subprocess
import sys
import termios
import time
import urllib.request

ANSI = re.compile(r"\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[()][A-Z0-9]|\x1b[=>]")


class Children:
    """Register ownership before unmasking interruption, including fork setup."""
    def __init__(self):
        self.children = {}

    def spawn(self, argv, **kwargs):
        old = signal.pthread_sigmask(signal.SIG_BLOCK, {signal.SIGINT, signal.SIGTERM})
        try:
            process = subprocess.Popen(argv, start_new_session=True,
                preexec_fn=lambda: signal.pthread_sigmask(signal.SIG_SETMASK, old), **kwargs)
            self.children[process.pid] = process
            return process
        finally:
            signal.pthread_sigmask(signal.SIG_SETMASK, old)

    def fork_pty(self, argv, env, cwd):
        old = signal.pthread_sigmask(signal.SIG_BLOCK, {signal.SIGINT, signal.SIGTERM})
        try:
            pid, fd = pty.fork()
            if pid == 0:
                try:
                    signal.pthread_sigmask(signal.SIG_SETMASK, old)
                    fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 160, 0, 0))
                    os.chdir(cwd)
                    os.execve(argv[0], argv, env)
                finally:
                    os._exit(127)
            self.children[pid] = None
            return pid, fd
        finally:
            signal.pthread_sigmask(signal.SIG_SETMASK, old)

    def wait(self, pid, timeout=5):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            process = self.children[pid]
            if process is not None:
                status = process.poll()
                if status is not None:
                    del self.children[pid]
                    return status
            else:
                done, status = os.waitpid(pid, os.WNOHANG)
                if done:
                    del self.children[pid]
                    return os.waitstatus_to_exitcode(status)
            time.sleep(0.01)
        raise AssertionError('owned child did not exit')

    def close(self):
        # Entire driver additionally lives in a die-with-parent PID namespace;
        # namespace teardown is the independent guard against hard parent death.
        for pid in list(self.children):
            try:
                os.killpg(pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
        for pid in list(self.children):
            try:
                self.wait(pid, 0.5)
            except AssertionError:
                try:
                    os.killpg(pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                self.wait(pid, 3)


def fixture_preflight(root):
    # Namespace root is sealed, host ancestors do not exist here. Fixture-only
    # metadata checks also reject context/project injection and symlink escapes.
    assert os.environ.get('ATUIN_E2E_NAMESPACE') == '1' and str(root) == '/fixture', 'unconfined driver denied'
    assert os.stat('/').st_dev != os.stat(root).st_dev, 'fixture is not a separate mount'
    assert stat.S_IMODE(root.stat().st_mode) == 0o700, 'fixture base not private'
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in dirs + files:
            path = pathlib.Path(directory) / name
            assert not path.is_symlink(), 'fixture symlink denied'
            assert name not in ('TERMINAL.md', '.git'), 'unapproved fixture context/project discovery denied'


def run(plan, children):
    root = pathlib.Path(plan['root'])
    fixture_preflight(root)
    for d in ('home', 'config', 'data', 'cache', 'state', 'runtime', 'tmp', 'atuin', 'sandbox'):
        (root / d).mkdir(mode=0o700, parents=True, exist_ok=True)
    config = '\n'.join([
        'auto_sync = false', 'update_check = false', '[logs]', 'enabled = false',
        '[daemon]', 'enabled = ' + ('true' if plan.get('daemon') else 'false'),
        'autostart = false', 'sync_frequency = 0', 'systemd_socket = false',
        'socket_path = ' + json.dumps(str(root / 'runtime' / 'atuin.sock')),
        'pidfile_path = ' + json.dumps(str(root / 'runtime' / 'daemon.pid')),
        '[output]', 'enabled = ' + ('true' if plan.get('daemon') else 'false'),
        '[ai]', 'enabled = true', 'endpoint_protocol = "oss"',
        'endpoint = ' + json.dumps(plan['endpoint']), 'session_continue_minutes = 30', 'tips = false',
    ]) + '\n'
    (root / 'atuin' / 'config.toml').write_text(config)
    skill = root / 'atuin' / 'skills' / 'synthetic'
    skill.mkdir(parents=True, exist_ok=True)
    (skill / 'SKILL.md').write_text('---\nname: synthetic\ndescription: synthetic fixture only\n---\nSYNTHETIC_SKILL_BODY\n')
    (root / 'sandbox' / 'fixture.txt').write_text('SYNTHETIC_OLD\n')
    env = {'PATH': '/usr/bin:/bin', 'TERM': 'xterm-256color', 'LANG': 'C.UTF-8',
        'SHELL': '/bin/bash', 'HOME': str(root / 'home'), 'ATUIN_HOME': str(root / 'atuin'),
        'XDG_CONFIG_HOME': str(root / 'config'), 'XDG_DATA_HOME': str(root / 'data'),
        'XDG_CACHE_HOME': str(root / 'cache'), 'XDG_STATE_HOME': str(root / 'state'),
        'XDG_RUNTIME_DIR': str(root / 'runtime'), 'TMPDIR': str(root / 'tmp'),
        'HTTP_PROXY': 'http://127.0.0.1:9', 'HTTPS_PROXY': 'http://127.0.0.1:9',
        'ALL_PROXY': 'http://127.0.0.1:9', 'NO_PROXY': '127.0.0.1',
        'GIT_CONFIG_NOSYSTEM': '1', 'ATUIN_SESSION': '00000000000000000000000000000001'}

    def cli(args):
        fixture_preflight(root)
        process = children.spawn([plan['binary']] + args, env=env, cwd=root / 'sandbox', stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        out, err = process.communicate(timeout=10)
        status = children.wait(process.pid)
        assert status == 0, 'isolated CLI preparation failed: ' + err.decode('utf-8', 'replace')[:2000]
        return out.decode().strip()

    # Synthetic cleanup regressions stop at explicit child-owned handshakes;
    # these use /bin/sh, not the actual client or context discovery.
    if plan.get('test_pause') == 'preparation':
        process = children.spawn(['/bin/sh', '-c', 'echo PREPARATION_READY; exec sleep 60'], env=env, stdout=subprocess.PIPE)
        assert process.stdout.readline() == b'PREPARATION_READY\n'
        print('PREPARATION_READY', flush=True)
        process.wait()
        return
    if plan.get('test_pause') == 'pty_setup':
        pid, fd = children.fork_pty(['/bin/sh', '-c', 'echo PTY_SETUP_READY; exec sleep 60'], env, root / 'sandbox')
        try:
            assert b'PTY_SETUP_READY' in os.read(fd, 1024)
            print('PTY_SETUP_READY', flush=True)
            children.wait(pid, 60)
        finally:
            os.close(fd)
        return

    if plan.get('daemon'):
        cli(['history', 'list', '--cmd-only'])
        fixture_preflight(root)
        daemon = children.spawn([plan['binary'], 'daemon', 'start'], env=env, cwd=root / 'sandbox', stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        deadline = time.monotonic() + 8
        while True:
            status = cli(['daemon', 'status'])
            if status.startswith('Daemon running\n') and re.search(r'Healthy:\s+true', status):
                observed = re.search(r'PID:\s+(\d+)', status)
                assert observed and int(observed.group(1)) == daemon.pid, 'non-owned daemon'
                assert str(root / 'runtime' / 'atuin.sock') in status, 'socket escaped fixture'
                break
            assert time.monotonic() < deadline, 'daemon not ready'
            time.sleep(0.1)
    if plan.get('seed_history') or plan.get('daemon'):
        history_id = cli(['history', 'start', '--', 'printf SYNTHETIC_HISTORY'])
        assert re.fullmatch(r'[0-9a-fA-F-]{32,36}', history_id), 'invalid fixture history ID'
        cli(['history', 'end', '--exit', '0', '--duration', '1000000', history_id])
        (root / 'history-id.txt').write_text(history_id)
    if plan.get('daemon'):
        capture = root / 'capture.sh'
        capture.write_text("printf '\\033]18188735;C;" + history_id + "\\007'\nprintf 'SYNTHETIC_CAPTURED_OUTPUT\\n'\nprintf '\\033]18188735;D;" + history_id + "\\007'\nread -r _\n")
        fixture_preflight(root)
        pid, fd = children.fork_pty([plan['binary'], 'pty-proxy', '--shell=/bin/bash', '--shell=--noprofile', '--shell=--norc', '--shell=' + str(capture)], env, root / 'sandbox')
        try:
            deadline, captured = time.monotonic() + 10, False
            while time.monotonic() < deadline:
                if not captured:
                    result = cli(['output', 'search', '--style', 'json', 'SYNTHETIC_CAPTURED_OUTPUT'])
                    if history_id in result and 'SYNTHETIC_CAPTURED_OUTPUT' in result:
                        captured = True
                        os.write(fd, b'\n')
                if select.select([fd], [], [], 0.1)[0]:
                    try:
                        if not os.read(fd, 65536):
                            break
                    except OSError as error:
                        if error.errno == errno.EIO:
                            break
                        raise
            else:
                raise AssertionError('capture did not exit')
            assert captured and children.wait(pid) == 0, 'capture failed'
        finally:
            os.close(fd)

    fixture_preflight(root)
    pid, fd = children.fork_pty([plan['binary'], 'ai', 'inline', '--hook', plan.get('prompt', 'synthetic question')], env, root / 'sandbox')
    raw, alive, query_tail = bytearray(), True, b''

    def pump(timeout):
        nonlocal alive, query_tail
        if select.select([fd], [], [], timeout)[0]:
            try:
                data = os.read(fd, 65536)
            except OSError as error:
                if error.errno != errno.EIO:
                    raise
                data = b''
            if not data:
                alive = False
                return
            raw.extend(data)
            assert len(raw) <= 2 * 1024 * 1024, 'PTY output bound'
            queries = (query_tail + data).count(b'\x1b[6n')
            query_tail = (query_tail + data)[-3:]
            if queries:
                os.write(fd, b'\x1b[1;1R' * queries)

    try:
        fresh_start = 0
        for step in plan['steps']:
            if 'wait' in step:
                start = fresh_start if step.get('fresh') else 0
                deadline = time.monotonic() + step.get('timeout', 20)
                while not re.search(step['wait'], ANSI.sub('', raw[start:].decode('utf-8', 'replace'))):
                    assert alive and time.monotonic() < deadline, 'missing client observable: ' + step['wait']
                    pump(0.1)
            if 'send' in step:
                os.write(fd, step['send'].encode())
            if 'absent' in step:
                assert not (root / 'sandbox' / step['absent']).exists(), 'premature tool execution'
            if step.get('release'):
                fresh_start = len(raw)  # Later permission match must be new output.
                with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(plan['endpoint'] + '/__release', timeout=3) as response:
                    assert response.status == 204
        deadline = time.monotonic() + 5
        while alive and time.monotonic() < deadline:
            pump(0.1)
        assert not alive and children.wait(pid) == 0, 'CLI did not exit successfully'
        assert plan.get('expected_result', '__atuin_ai_cancel__').encode() in raw, 'wrong hook outcome'
        assert b'__atuin_ai_execute__:' not in raw, 'suggestion executed'
        for secret in (b'synthetic-private-detail', b'fake-refresh', b'FAKE-ONLY', b'synthetic-account'):
            assert secret not in raw, 'fake private state leaked'
        print(json.dumps({'result': 'passed', 'observables': [s['wait'] for s in plan['steps'] if 'wait' in s]}))
    except BaseException:
        print(ANSI.sub('', raw.decode('utf-8', 'replace'))[-6000:], file=sys.stderr)
        raise
    finally:
        os.close(fd)


def main():
    def interrupted(_signal, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)  # background launch may inherit SIG_IGN
    children = Children()
    try:
        run(json.load(sys.stdin), children)
    finally:
        children.close()


if __name__ == '__main__':
    main()
