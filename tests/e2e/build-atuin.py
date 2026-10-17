#!/usr/bin/env python3
"""Pinned unmodified Atuin build. Fresh source/toolchain/Cargo/target every run.
Usage: python3 tests/e2e/build-atuin.py PINNED_GIT TASK_OWNED_0700_ROOT
Only checksum-verified official download archives may be cached. A local
manifest is consistency evidence, NOT independent producer attestation.
"""
import json
import os
import pathlib
import shutil
import signal
import subprocess
import sys
import tarfile
import time

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
from build_support import cached_archive, checked_repo, digest, fresh_archive, inventory, run_directory, verify_inventory

from bwrap_tool import resolve

PIN = '4ca8b77c8a526b8c294eb864a2b74c95c685c983'
RUST = '1.98.0'
COMPONENTS = {
    'rustc': '0e37cb339f447fc44d6d781073bacacebfdc5612f2600e4c7e84c266f5f3aced',
    'cargo': '2f512d170d3dd23e16ababcda32ee2e6d5172d861a7af1f504e0b1e270cafab9',
    'rust-std': 'f5022e6c95a5ad23cca2513dc8281200f585fa188de6370aa37b128a43f876a3',
}
FLAGS = {'CARGO_BUILD_JOBS': '2', 'CARGO_INCREMENTAL': '0', 'CARGO_PROFILE_RELEASE_DEBUG': '0',
         'CARGO_PROFILE_RELEASE_STRIP': 'symbols', 'CARGO_PROFILE_RELEASE_LTO': 'false'}


def build(args, root, env):
    process = subprocess.Popen(args, env=env, start_new_session=True)
    try:
        deadline = time.monotonic() + 1800
        while process.poll() is None:
            if time.monotonic() >= deadline or shutil.disk_usage(root).free < 1024**3:
                raise RuntimeError('build time/disk budget exhausted')
            time.sleep(2)
        if process.returncode:
            raise RuntimeError('pinned client build failed')
    finally:
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=10)


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    repo, root = [pathlib.Path(p).absolute() for p in sys.argv[1:]]
    work = run_directory(root)  # invalidates old manifest before any validation
    checked_repo(repo, PIN)
    if shutil.disk_usage(root).free < 5 * 1024**3:
        raise RuntimeError('need >=5GiB free for fresh bounded build')
    source = work / 'source'
    archive_hash, expected = fresh_archive(repo, PIN, source)
    for name in ('home', 'cargo', 'target', 'toolchain', 'install'):
        (work / name).mkdir(mode=0o700)
    downloads = root / 'downloads'
    downloads.mkdir(mode=0o700, exist_ok=True)
    install_env = {'HOME': str(work / 'home'), 'PATH': '/usr/bin:/bin'}
    for name, checksum in COMPONENTS.items():
        filename = f'{name}-{RUST}-x86_64-unknown-linux-gnu.tar.xz'
        archive = cached_archive(downloads, filename, 'https://static.rust-lang.org/dist/2026-08-20/' + filename, checksum)
        with tarfile.open(archive) as tar:
            tar.extractall(work / 'install', filter='data')
        extracted = work / 'install' / filename[:-7]
        subprocess.run([str(extracted / 'install.sh'), '--prefix=' + str(work / 'toolchain'), '--disable-ldconfig'], env=install_env, check=True, timeout=180)
        shutil.rmtree(extracted)
    toolchain_expected = inventory(work / 'toolchain')
    verify_inventory(source, expected)
    # No host HOME, Cargo configuration, source ancestor, compiler wrapper,
    # target or registry cache is visible to Cargo. Source .cargo is pinned.
    env = dict(FLAGS, HOME='/work/home', CARGO_HOME='/work/cargo', CARGO_TARGET_DIR='/work/target',
               RUSTC='/toolchain/bin/rustc', PATH='/toolchain/bin:/usr/bin:/bin',
               CC='/usr/bin/gcc', CXX='/usr/bin/g++', CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER='/usr/bin/gcc')
    args = [resolve(), '--unshare-user', '--unshare-pid', '--die-with-parent', '--cap-drop', 'ALL',
            '--ro-bind', '/usr', '/usr', '--symlink', 'usr/bin', '/bin', '--symlink', 'usr/lib', '/lib',
            '--symlink', 'usr/lib64', '/lib64', '--ro-bind', '/etc/ssl/certs', '/etc/ssl/certs',
            '--ro-bind', '/etc/ssl/openssl.cnf', '/etc/ssl/openssl.cnf',
            '--ro-bind', '/etc/resolv.conf', '/etc/resolv.conf', '--ro-bind', '/etc/hosts', '/etc/hosts',
            '--proc', '/proc', '--dev', '/dev', '--bind', str(work), '/work',
            '--ro-bind', str(work / 'toolchain'), '/toolchain', '--chdir', '/work/source', '--clearenv']
    for key, value in env.items():
        args += ['--setenv', key, value]
    args += ['/toolchain/bin/cargo', 'build', '--locked', '--release', '-p', 'atuin']
    build(args, root, {'PATH': '/usr/bin:/bin', 'HOME': str(work / 'home')})
    verify_inventory(source, expected)
    verify_inventory(work / 'toolchain', toolchain_expected)
    checked_repo(repo, PIN)
    binary = work / 'target' / 'release' / 'atuin'
    manifest = dict(source_sha=PIN, rust=RUST, binary=str(binary), binary_sha256=digest(binary),
        source_archive_sha256=archive_hash, cargo_lock_sha256=digest(source / 'Cargo.lock'),
        command=['cargo', 'build', '--locked', '--release', '-p', 'atuin'], flags=FLAGS, build_env=env,
        recipe_sha256=digest(pathlib.Path(__file__)), support_sha256=digest(pathlib.Path(__file__).resolve().parents[1] / 'build_support.py'), toolchain_components=COMPONENTS,
        fresh_inputs=True, source_worktree_unchanged=True)
    (root / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print('ATUIN_E2E_MANIFEST=' + str(root / 'manifest.json'))


if __name__ == '__main__':
    main()
