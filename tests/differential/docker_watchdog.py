#!/usr/bin/env python3
"""Independent lease cleanup after stdin EOF / parent death, OR workflow always.
Only resources bearing the exact Phase8 run (and optional lease) label may be
removed. Never prune, enumerate/delete unlabeled names, or touch images/caches.
"""
import os
import re
import select
import signal
import subprocess
import sys
import time

RUN_KEY = 'ai.atuin.phase8.run'
LEASE_KEY = 'ai.atuin.phase8.lease'


def cleanup(run, lease=None):
    if not re.fullmatch(r'atuin-phase8-[a-zA-Z0-9_.-]{1,100}', run):
        raise ValueError('invalid owned run label')
    if lease is not None and not re.fullmatch(r'[a-f0-9]{24}', lease):
        raise ValueError('invalid lease')
    docker = os.environ.get('ATUIN_PHASE8_DOCKER', '/usr/bin/docker')
    if not os.path.isabs(docker):
        raise ValueError('docker executable must be absolute')
    filters = ['--filter', 'label=' + RUN_KEY + '=' + run]
    if lease is not None:
        filters += ['--filter', 'label=' + LEASE_KEY + '=' + lease]
    errors = []
    for kind, listing, removing in [('containers', ['ps', '-aq'], ['rm', '-f']), ('networks', ['network', 'ls', '-q'], ['network', 'rm'])]:
        result = subprocess.run([docker, '--host=unix:///var/run/docker.sock'] + listing + filters, capture_output=True, text=True, timeout=10)
        if result.returncode:
            errors.append(kind + ' listing failed')
            continue
        for resource in result.stdout.split():
            if not re.fullmatch(r'[a-f0-9]{12,64}', resource):
                raise ValueError('invalid scoped resource ID')
            deleted = subprocess.run([docker, '--host=unix:///var/run/docker.sock'] + removing + [resource], capture_output=True, timeout=10)
            if deleted.returncode:
                errors.append(kind + ' scoped deletion failed')
    if errors:
        raise RuntimeError('; '.join(errors))


def main():
    stopped = [False]
    def interrupted(_signal, _frame):
        stopped[0] = True
    signal.signal(signal.SIGINT, interrupted)
    signal.signal(signal.SIGTERM, interrupted)
    if len(sys.argv) not in (3, 4) or sys.argv[1] not in ('watch', 'cleanup'):
        sys.exit('usage: docker_watchdog.py watch|cleanup RUN [LEASE]')
    if sys.argv[1] == 'watch':
        # Independent process, not CommandContext/t.Cleanup. A killed/timeout Go
        # process closes its sole writer; even a create that succeeded remotely
        # before its client timed out is now found by labels and removed.
        deadline = time.monotonic() + 15 * 60
        pending, buffered = 0, b''
        while time.monotonic() < deadline and not stopped[0]:
            if select.select([sys.stdin], [], [], 1)[0]:
                data = os.read(0, 4096)
                if not data:
                    break
                buffered += data
                while b'\n' in buffered:
                    line, buffered = buffered.split(b'\n', 1)
                    if line == b'begin':
                        pending += 1
                    elif line == b'end':
                        pending -= 1
                    else:
                        raise ValueError('invalid lease protocol')
        # A timed-out/interrupted create can finish remotely after its CLI
        # disappeared. Keep draining labels for a bounded late-create window.
        if pending:
            grace = float(os.environ.get('ATUIN_PHASE8_TEST_GRACE', '30'))
            if not 1 <= grace <= 30:
                raise ValueError('invalid late-create cleanup budget')
            until = time.monotonic() + grace
            while time.monotonic() < until:
                cleanup(sys.argv[2], sys.argv[3] if len(sys.argv) == 4 else None)
                time.sleep(0.2)
    cleanup(sys.argv[2], sys.argv[3] if len(sys.argv) == 4 else None)


if __name__ == '__main__':
    main()
