"""Test build integrity helpers; no application/runtime import."""
import hashlib
import os
import pathlib
import shutil
import stat
import subprocess
import tarfile
import tempfile
import urllib.request


def digest(path, algorithm='sha256'):
    with open(path, 'rb') as stream:
        return hashlib.file_digest(stream, algorithm).hexdigest()


def cached_archive(directory, filename, url, expected):
    path = directory / filename
    if directory.is_symlink() or (directory.stat().st_mode & 0o077):
        raise ValueError('unsafe download cache directory')
    if path.is_symlink() or (path.exists() and (not path.is_file() or path.stat().st_nlink != 1)):
        raise ValueError('symlinked/hardlinked download cache denied')
    if not path.exists():
        with urllib.request.urlopen(url, timeout=60) as response, tempfile.NamedTemporaryFile(dir=directory, delete=False) as out:
            temporary = pathlib.Path(out.name)
            try:
                shutil.copyfileobj(response, out)
                out.close()
                if digest(temporary) != expected:
                    raise ValueError('official archive checksum mismatch')
                os.replace(temporary, path)
            finally:
                temporary.unlink(missing_ok=True)
    if digest(path) != expected:
        raise ValueError('corrupt cached archive; refusing use (no marker bypass)')
    return path


def inventory(root):
    result = {}
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in dirs + files:
            path = pathlib.Path(directory) / name
            info = path.lstat()
            relative = path.relative_to(root).as_posix()
            mode = stat.S_IMODE(info.st_mode)
            if stat.S_ISLNK(info.st_mode):
                value = ('link', os.readlink(path))
            elif stat.S_ISREG(info.st_mode):
                value = ('file', mode, digest(path))
            elif stat.S_ISDIR(info.st_mode):
                value = ('dir', mode)
            else:
                raise ValueError('unexpected source file type: ' + relative)
            result[relative] = value
    return result


def fresh_archive(repo, pin, destination):
    """Never overlay old context; establish complete pinned extraction inventory."""
    if destination.exists() or destination.is_symlink():
        raise ValueError('source destination must be fresh')
    archive = destination.with_suffix('.tar')
    with archive.open('wb') as stream:
        subprocess.run(['git', '-C', str(repo), 'archive', pin], stdout=stream, check=True, timeout=60)
    archive_hash = digest(archive)
    destination.mkdir(mode=0o700)
    with tarfile.open(archive) as tar:
        tar.extractall(destination, filter='data')
    # Expected complete inventory includes directory entries and executable bits,
    # not only known tracked file bytes. Fresh materialization prevents extras.
    expected = inventory(destination)
    archive.unlink()
    return archive_hash, expected


def verify_inventory(root, expected):
    if inventory(root) != expected:
        raise ValueError('source inventory changed (added/removed/modified/symlink/mode)')


def checked_repo(repo, pin):
    if subprocess.check_output(['git', '-C', str(repo), 'rev-parse', 'HEAD'], text=True).strip() != pin:
        raise ValueError('wrong pinned source')
    if subprocess.check_output(['git', '-C', str(repo), 'status', '--porcelain']):
        raise ValueError('dirty pinned source')


def reject_config(root):
    for path in (root / 'cargo' / 'config', root / 'cargo' / 'config.toml',
                 root / '.cargo' / 'config', root / '.cargo' / 'config.toml',
                 root / 'home' / '.cargo'):
        if path.exists() or path.is_symlink():
            raise ValueError('untrusted retained Cargo configuration denied')


def run_directory(root):
    if root.is_symlink():
        raise ValueError('symlinked build root denied')
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    if root.stat().st_uid != os.geteuid() or stat.S_IMODE(root.stat().st_mode) != 0o700:
        raise ValueError('build root must be private owned 0700')
    # Invalidate before any operation that can fail. Stale success cannot survive.
    (root / 'manifest.json').unlink(missing_ok=True)
    reject_config(root)
    runs = root / 'runs'
    runs.mkdir(mode=0o700, exist_ok=True)
    if runs.is_symlink() or runs.stat().st_uid != os.geteuid() or stat.S_IMODE(runs.stat().st_mode) != 0o700:
        raise ValueError('unsafe retained runs directory')
    return pathlib.Path(tempfile.mkdtemp(prefix='build-', dir=runs))
