"""Test-only verified Bubblewrap selection; no runtime policy/configuration knob."""
import json
import os
import pathlib
import stat
import subprocess
from build_support import digest, inventory

VERSION = '0.11.1'
ARCHIVE_SHA256 = 'c1b7455a1283b1295879a46d5f001dfd088c0bb0f238abb5e128b3583a246f71'
VARIABLE = 'ATUIN_TEST_BWRAP_MANIFEST'


def resolve(env=None):
    env = os.environ if env is None else env
    name = env.get(VARIABLE)
    if not name:
        return '/usr/bin/bwrap'  # unchanged locally pre-reviewed system prerequisite
    manifest = pathlib.Path(name)
    if not manifest.is_absolute() or manifest.is_symlink():
        raise ValueError('Bubblewrap manifest must be an absolute regular file')
    data = json.loads(manifest.read_text())
    base = manifest.parent
    prefix = base / 'prefix'
    binary = prefix / 'bin' / 'bwrap'
    for path in (base, prefix):
        info=path.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o700:
            raise ValueError('Bubblewrap prefix must be owned/private')
    here = pathlib.Path(__file__).resolve().parent
    if (data.get('version') != VERSION or data.get('archive_sha256') != ARCHIVE_SHA256
            or data.get('builder_sha256') != digest(here / 'build-bwrap.py')
            or data.get('selector_sha256') != digest(here / 'bwrap_tool.py')
            or data.get('support_sha256') != digest(here / 'build_support.py')
            or data.get('source_unchanged') is not True
            or data.get('inventory') != json.loads(json.dumps(inventory(prefix)))):
        raise ValueError('Bubblewrap source/recipe/prefix inventory mismatch')
    info=binary.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o6000 or not info.st_mode & stat.S_IXUSR:
        raise ValueError('Bubblewrap must be regular executable, never setuid/setgid')
    result=subprocess.run([str(binary),'--version'],env={'PATH':'/usr/bin:/bin','HOME':str(base)},capture_output=True,text=True,check=True,timeout=5)
    if result.stdout.strip() != 'bubblewrap '+VERSION:
        raise ValueError('Bubblewrap executable version mismatch')
    return str(binary)
