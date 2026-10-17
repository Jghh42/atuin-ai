#!/usr/bin/env python3
"""Owned-prefix official Bubblewrap build. Dependencies are external OS tools.

No developer-host install, setuid bit, sysctl or AppArmor change. CI profile
setup is separately explicit/reviewable. Success proves local consistency, not
independent producer attestation. All source bytes come from the pinned release.
"""
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tarfile
import tempfile
from build_support import cached_archive, digest, inventory, verify_inventory
from bwrap_tool import ARCHIVE_SHA256, VERSION, VARIABLE, resolve


def build(root):
    root=pathlib.Path(root).absolute()
    if root.exists():
        if root.is_symlink() or root.stat().st_uid!=os.geteuid() or root.stat().st_mode & 0o077:
            raise ValueError('build root must be owned/private')
    else: root.mkdir(mode=0o700,parents=True)
    manifest=root/'manifest.json'
    manifest.unlink(missing_ok=True)
    if (root/'prefix').exists():
        raise ValueError('fresh prefix required; never overlay a tool installation')
    downloads=root/'downloads';downloads.mkdir(mode=0o700,exist_ok=True)
    archive=cached_archive(downloads,f'bubblewrap-{VERSION}.tar.xz',f'https://github.com/containers/bubblewrap/releases/download/v{VERSION}/bubblewrap-{VERSION}.tar.xz',ARCHIVE_SHA256)
    work=pathlib.Path(tempfile.mkdtemp(prefix='build-',dir=root))
    with tarfile.open(archive) as tar: tar.extractall(work,filter='data')
    source=work/f'bubblewrap-{VERSION}'
    expected=inventory(source)
    prefix=root/'prefix';prefix.mkdir(mode=0o700)
    home=work/'home';home.mkdir(mode=0o700)
    env={'PATH':os.environ.get('PATH','/usr/bin:/bin'),'HOME':str(home),'LANG':'C.UTF-8','CC':'/usr/bin/gcc'}
    # Explicit developer-owned extracted OS build dependencies only; never carry
    # generic compiler wrappers, proxy credentials or user configuration through.
    for key in ('PKG_CONFIG_PATH','PKG_CONFIG_SYSROOT_DIR','PYTHONPATH'):
        if os.environ.get(key): env[key]=os.environ[key]
    log=[]
    def run(args):
        log.append(args)
        subprocess.run(args,env=env,check=True,timeout=180)
    flags=['--buildtype=release','--wrap-mode=nodownload','-Drequire_userns=true','-Dman=disabled','-Dtests=false','-Dbash_completion=disabled','-Dzsh_completion=disabled']
    run(['meson','setup',str(work/'build'),str(source),'--prefix='+str(prefix)]+flags)
    run(['meson','compile','-C',str(work/'build'),'-j','2'])
    run(['meson','install','-C',str(work/'build'),'--no-rebuild'])
    licenses=prefix/'share'/'licenses'/'bubblewrap';licenses.mkdir(parents=True)
    shutil.copyfile(source/'COPYING',licenses/'COPYING')
    shutil.copyfile(source/'README.md',licenses/'README.md')
    verify_inventory(source,expected)
    here=pathlib.Path(__file__).resolve().parent
    tool_records={}
    for tool in ('gcc','meson','ninja','pkg-config'):
        path=pathlib.Path(shutil.which(tool,path=env['PATH'])).resolve()
        tool_records[tool]={'path':str(path),'sha256':digest(path),'version':subprocess.check_output([str(path),'--version'],env=env,text=True,timeout=5).splitlines()[0]}
    data=dict(version=VERSION,archive_sha256=ARCHIVE_SHA256,builder_sha256=digest(pathlib.Path(__file__)),selector_sha256=digest(here/'bwrap_tool.py'),support_sha256=digest(here/'build_support.py'),source_unchanged=True,inventory=inventory(prefix),commands=log,build_tools=tool_records,dependency_env={k:v for k,v in env.items() if k in ('PKG_CONFIG_PATH','PKG_CONFIG_SYSROOT_DIR','PYTHONPATH')})
    manifest.write_text(json.dumps(data,indent=2)+'\n')
    try: resolve({VARIABLE:str(manifest)})
    except BaseException:
        manifest.unlink(missing_ok=True)
        raise
    print(VARIABLE+'='+str(manifest))


def probe(manifest):
    # Same fd mounts, namespaces, synthetic system data and readonly root as the
    # actual CLI; this is a prerequisite probe, never a replacement CLI gate.
    os.environ[VARIABLE]=str(pathlib.Path(manifest).absolute())
    sys.path.insert(0,str(pathlib.Path(__file__).parent/'e2e'))
    import confinement
    with tempfile.TemporaryDirectory(prefix='namespace-probe-') as tmp:
        root=pathlib.Path(tmp);root.chmod(0o700)
        (root/'marker').touch()
        driver=root/'probe.py'
        driver.write_text('import os,pathlib,socket\nassert os.getpid()>1\nassert socket.gethostname()=="fixture"\nassert pathlib.Path("/fixture/marker").exists()\nassert not pathlib.Path('+repr(tmp)+').exists()\nassert not pathlib.Path("/home").exists()\nassert pathlib.Path("/proc/self/ns/pid").exists()\nprint("PINNED_NAMESPACE_OK")\n')
        args,fds=confinement.command(str(root),str(pathlib.Path('/usr/bin/true').resolve()),str(driver),{'PATH':'/usr/bin:/bin','HOME':'/fixture/home'})
        try: subprocess.run(args,pass_fds=fds,env={'PATH':'/usr/bin:/bin'},check=True,timeout=10)
        finally:
            for fd in fds: os.close(fd)


if __name__=='__main__':
    if len(sys.argv)==3 and sys.argv[1]=='probe': probe(sys.argv[2])
    elif len(sys.argv)==3 and sys.argv[1]=='resolve': print(resolve({VARIABLE:str(pathlib.Path(sys.argv[2]).absolute())}))
    elif len(sys.argv)==2: build(sys.argv[1])
    else: raise SystemExit('usage: build-bwrap.py OWNED_NEW_ROOT | probe MANIFEST | resolve MANIFEST')
