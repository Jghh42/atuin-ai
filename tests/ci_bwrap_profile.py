#!/usr/bin/env python3
"""Hosted ephemeral CI ONLY: exact-path userns permission, never global sysctl.

The optional AppArmor profile grants userns to ONLY this job's verified
non-setuid bwrap. It neither disables AppArmor nor grants root to test children.
Kernel profile removal is a mandatory always-step; no /etc profile is installed.
"""
import json
import os
import pathlib
import re
import subprocess
import sys
from bwrap_tool import resolve, VARIABLE


def job(env):
    if env.get('GITHUB_ACTIONS')!='true' or env.get('RUNNER_ENVIRONMENT')!='github-hosted':
        raise ValueError('profile setup/removal permitted only on ephemeral GitHub-hosted CI')
    ids=[env.get('GITHUB_RUN_ID',''),env.get('GITHUB_RUN_ATTEMPT','')]
    if not all(re.fullmatch(r'[0-9]+',v) for v in ids): raise ValueError('invalid job identity')
    root=pathlib.Path(env['RUNNER_TEMP']).resolve(strict=True)
    name='atuin_phase8_bwrap_'+'_'.join(ids)
    return root,name,root/(name+'.profile'),root/(name+'.json')


def profile(binary,name,root):
    path=pathlib.Path(binary)
    if not path.is_absolute() or '..' in path.parts or not path.is_relative_to(root) or not re.fullmatch(r'[A-Za-z0-9_/.-]+',str(path)):
        raise ValueError('profile executable must be a safe exact job-owned path')
    return 'abi <abi/4.0>,\ninclude <tunables/global>\nprofile '+name+' "'+str(path)+'" flags=(unconfined) {\n  userns,\n}\n'


def configure(mode,env,restriction=pathlib.Path('/proc/sys/kernel/apparmor_restrict_unprivileged_userns')):
    root,name,path,receipt=job(env)
    if mode=='install':
        if path.exists() or receipt.exists(): raise ValueError('profile receipt must be fresh')
        binary=resolve(env)
        text=profile(binary,name,root)
        restricted=restriction.exists() and restriction.read_text().strip()=='1'
        receipt.write_text(json.dumps({'binary':binary,'profile_required':restricted})+'\n')
        receipt.chmod(0o600)
        if not restricted:
            print('No AppArmor unprivileged-userns restriction; no privileged setup')
            return
        # Write ownership/cleanup receipt BEFORE the privileged operation. A
        # failed/partial install still reaches the always removal attempt.
        path.write_text(text);path.chmod(0o600)
        print('Loading exact-path job AppArmor userns permission: '+name,flush=True)
        subprocess.run(['sudo','-n','/usr/sbin/apparmor_parser','-r','-T',str(path)],check=True,timeout=20)
    elif mode=='remove':
        if not receipt.exists(): return
        data=json.loads(receipt.read_text())
        expected=profile(data['binary'],name,root)
        if data['profile_required']:
            if path.is_symlink() or path.read_text()!=expected: raise ValueError('profile cleanup identity changed')
            subprocess.run(['sudo','-n','/usr/sbin/apparmor_parser','-R',str(path)],check=True,timeout=20)
            path.unlink()
        receipt.unlink()
    else: raise ValueError('install or remove required')


if __name__=='__main__':
    if len(sys.argv)!=2: raise SystemExit('usage: ci_bwrap_profile.py install|remove')
    configure(sys.argv[1],os.environ)
