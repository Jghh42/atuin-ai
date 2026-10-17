#!/usr/bin/env python3
"""Linux filesystem/PID namespace boundary for every actual Atuin launch.

Only the pinned executable, this test driver, system /usr, and an fd-pinned
private fixture directory are exposed. Host ancestors, HOME, /tmp and /etc are
NOT mounted. No metadata-check-then-exec race on a world-writable ancestor.
"""
import json
import os
import pathlib
import signal
import stat
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
from bwrap_tool import resolve


def directory_fd(path):
    path = pathlib.Path(path)
    if not path.is_absolute() or '..' in path.parts:
        raise ValueError('fixture must be absolute')
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for part in path.parts[1:]:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = child
        info = os.fstat(fd)
        if info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700:
            raise ValueError('fixture base must be owned/private 0700')
        return fd
    except BaseException:
        os.close(fd)
        raise


def command(root, executable, driver, env):
    # Bind FDs pin inodes before namespace creation; path ancestors may change
    # without exposing replacement directories or context files to the child.
    root_fd = directory_fd(root)
    binary_fd = os.open(executable, os.O_RDONLY | os.O_NOFOLLOW)
    driver_fd = os.open(driver, os.O_RDONLY | os.O_NOFOLLOW)
    fds = [root_fd, binary_fd, driver_fd]
    public_files = {
        '/etc/passwd': f'fixture:x:{os.geteuid()}:{os.getegid()}:Synthetic fixture:/fixture/home:/bin/bash\n',
        '/etc/group': f'fixture:x:{os.getegid()}:\n',
        '/etc/os-release': 'NAME=Fixture\nID=fixture\n',
        '/etc/nsswitch.conf': 'passwd: files\ngroup: files\nhosts: files\n',
    }
    public_fds=[]
    for path,contents in public_files.items():
        fd=os.memfd_create('synthetic-system-data',0)
        os.write(fd,contents.encode());os.lseek(fd,0,0)
        fds.append(fd);public_fds.append((path,fd))
    for fd in fds:
        os.set_inheritable(fd, True)
    args = [resolve(), '--unshare-user', '--unshare-pid', '--unshare-ipc',
            '--unshare-uts', '--hostname', 'fixture', '--die-with-parent',
            '--cap-drop', 'ALL', '--ro-bind', '/usr', '/usr',
            '--symlink', 'usr/bin', '/bin', '--symlink', 'usr/lib', '/lib',
            '--symlink', 'usr/lib64', '/lib64', '--proc', '/proc', '--dev', '/dev',
            '--dir', '/etc', '--dir', '/tmp', '--dir', '/fixture',
            '--bind-fd', str(root_fd), '/fixture',
            '--ro-bind-fd', str(binary_fd), '/atuin',
            '--ro-bind-fd', str(driver_fd), '/driver.py',
            '--chdir', '/fixture', '--clearenv']
    for path,fd in public_fds:
        args += ['--ro-bind-data',str(fd),path]
    args += ['--remount-ro','/']
    for key, value in env.items():
        args += ['--setenv', key, value]
    args += ['--setenv', 'ATUIN_E2E_NAMESPACE', '1', '/usr/bin/python3', '-I', '/driver.py']
    return args, fds


def main():
    plan = json.load(sys.stdin)
    driver = pathlib.Path(__file__).with_name('pty_driver.py')
    args, _ = command(plan['root'], plan['binary'], str(driver),
                      {'PATH': '/usr/bin:/bin', 'LANG': 'C.UTF-8', 'HOME': '/fixture/home'})
    plan['root'], plan['binary'] = '/fixture', '/atuin'
    # Feed rewritten plan through an anonymous fd, never a host context file.
    fd = os.memfd_create('synthetic-plan', 0)
    os.write(fd, json.dumps(plan).encode())
    os.lseek(fd, 0, 0)
    os.dup2(fd, 0)
    os.close(fd)
    signal.signal(signal.SIGINT, signal.SIG_DFL)
    signal.signal(signal.SIGTERM, signal.SIG_DFL)
    os.execve(args[0], args, {'PATH': '/usr/bin:/bin'})


if __name__ == '__main__':
    main()
