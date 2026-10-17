"""Offline regressions: build integrity, filesystem confinement, child cleanup."""
import hashlib
import importlib.util
import json
import os
import pathlib
import select
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock
from types import SimpleNamespace

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from build_support import cached_archive, fresh_archive, inventory, reject_config, run_directory, verify_inventory
spec = importlib.util.spec_from_file_location('confinement', pathlib.Path(__file__).parent / 'e2e' / 'confinement.py')
confinement = importlib.util.module_from_spec(spec)
spec.loader.exec_module(confinement)


class Integrity(unittest.TestCase):
    def test_extra_source_and_overlay_denied(self):
        with tempfile.TemporaryDirectory() as name:
            root = pathlib.Path(name)
            repo = root / 'repo'
            repo.mkdir()
            env = {'PATH': '/usr/bin:/bin', 'HOME': str(root), 'GIT_CONFIG_NOSYSTEM': '1'}
            subprocess.run(['git', 'init', '-q', str(repo)], env=env, check=True)
            (repo / 'lib.ex').write_text('synthetic original')
            subprocess.run(['git', '-C', str(repo), 'add', '.'], env=env, check=True)
            subprocess.run(['git', '-C', str(repo), '-c', 'user.name=fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-qm', 'fixture'], env=env, check=True)
            _, expected = fresh_archive(repo, 'HEAD', root / 'source')
            (root / 'source' / 'leftover.ex').write_text('poison must never compile')
            with self.assertRaises(ValueError):
                verify_inventory(root / 'source', expected)
            with self.assertRaises(ValueError):
                fresh_archive(repo, 'HEAD', root / 'source')
            (root / 'source' / 'leftover.ex').unlink()
            (root / 'source' / 'lib.ex').chmod(0o755)
            with self.assertRaises(ValueError):
                verify_inventory(root / 'source', expected)

    def test_cache_markers_and_configuration_not_trusted(self):
        with tempfile.TemporaryDirectory() as name:
            root = pathlib.Path(name)
            (root / 'archive').write_bytes(b'corrupt')
            (root / 'rustc.installed').touch()
            with self.assertRaises(ValueError):
                cached_archive(root, 'archive', 'https://invalid.invalid/NEVER_CONTACTED', hashlib.sha256(b'official').hexdigest())
            (root / 'cargo').mkdir()
            (root / 'cargo' / 'config.toml').write_text('[build]\nrustc-wrapper="poison"\n')
            (root / 'manifest.json').write_text('stale success')
            with self.assertRaises(ValueError):
                run_directory(root)
            self.assertFalse((root / 'manifest.json').exists())
            with self.assertRaises(ValueError):
                reject_config(root)


    def test_bwrap_manifest_inventory_version_and_recipe_fail_closed(self):
        import bwrap_tool
        from build_support import digest
        with tempfile.TemporaryDirectory() as name:
            root=pathlib.Path(name);root.chmod(0o700)
            prefix=root/'prefix';prefix.mkdir(mode=0o700);(prefix/'bin').mkdir()
            binary=prefix/'bin'/'bwrap';binary.write_bytes(b'SYNTHETIC_NOT_EXECUTED');binary.chmod(0o755)
            here=pathlib.Path(__file__).parent
            data=dict(version=bwrap_tool.VERSION,archive_sha256=bwrap_tool.ARCHIVE_SHA256,builder_sha256=digest(here/'build-bwrap.py'),selector_sha256=digest(here/'bwrap_tool.py'),support_sha256=digest(here/'build_support.py'),source_unchanged=True,inventory=inventory(prefix))
            manifest=root/'manifest.json';manifest.write_text(json.dumps(data))
            env={bwrap_tool.VARIABLE:str(manifest)}
            with mock.patch.object(bwrap_tool.subprocess,'run',return_value=SimpleNamespace(stdout='bubblewrap 0.11.1\n')) as run:
                self.assertEqual(bwrap_tool.resolve(env),str(binary)) # JSON inventory roundtrip
                self.assertEqual(run.call_count,1)
                for key,value in [('version','0.0'),('archive_sha256','wrong'),('builder_sha256','wrong'),('source_unchanged',False)]:
                    changed=dict(data);changed[key]=value;manifest.write_text(json.dumps(changed))
                    with self.assertRaises(ValueError):bwrap_tool.resolve(env)
                manifest.write_text(json.dumps(data))
                (prefix/'extra').touch()
                with self.assertRaises(ValueError):bwrap_tool.resolve(env)
                (prefix/'extra').unlink();binary.write_bytes(b'corrupt')
                with self.assertRaises(ValueError):bwrap_tool.resolve(env)
                binary.write_bytes(b'SYNTHETIC_NOT_EXECUTED');binary.chmod(0o4755)
                data['inventory']=inventory(prefix);manifest.write_text(json.dumps(data))
                with self.assertRaises(ValueError):bwrap_tool.resolve(env)
                binary.chmod(0o755);data['inventory']=inventory(prefix);manifest.write_text(json.dumps(data))
                run.return_value.stdout='bubblewrap WRONG\n'
                with self.assertRaises(ValueError):bwrap_tool.resolve(env)
            self.assertEqual(bwrap_tool.resolve({}),'/usr/bin/bwrap') # cleared default unchanged

    def test_ci_profile_is_job_only_scoped_and_removed_after_partial_setup(self):
        import ci_bwrap_profile as profile
        with self.assertRaises(ValueError):profile.configure('install',{})
        for restricted in (False,True):
            with self.subTest(restricted=restricted),tempfile.TemporaryDirectory() as name:
                root=pathlib.Path(name);restriction=root/'restriction';restriction.write_text('1' if restricted else '0')
                env={'GITHUB_ACTIONS':'true','RUNNER_ENVIRONMENT':'github-hosted','GITHUB_RUN_ID':'123','GITHUB_RUN_ATTEMPT':'2','RUNNER_TEMP':name}
                binary=str(root/'prefix'/'bin'/'bwrap')
                with mock.patch.object(profile,'resolve',return_value=binary),mock.patch.object(profile.subprocess,'run') as run:
                    if restricted:
                        run.side_effect=subprocess.CalledProcessError(1,'synthetic load failure')
                        with self.assertRaises(subprocess.CalledProcessError):profile.configure('install',env,restriction)
                        run.side_effect=None
                    else:profile.configure('install',env,restriction)
                    _,label,path,receipt=profile.job(env)
                    self.assertTrue(receipt.exists())
                    if restricted:
                        text=path.read_text();self.assertIn('"'+binary+'"',text);self.assertIn('userns,',text)
                        path.write_text(text.replace(label,'UNRELATED'))
                        with self.assertRaises(ValueError):profile.configure('remove',env,restriction)
                        path.write_text(text)
                    profile.configure('remove',env,restriction)
                    self.assertFalse(receipt.exists());self.assertFalse(path.exists())
                    if restricted:
                        self.assertEqual(run.call_count,2)
                        self.assertEqual(run.call_args.args[0],['sudo','-n','/usr/sbin/apparmor_parser','-R',str(path)])
                    else:run.assert_not_called()
                for invalid in ('/usr/bin/bwrap',str(root/'wild*')):
                    with self.assertRaises(ValueError):profile.profile(invalid,label,root)

class Confinement(unittest.TestCase):
    def launch(self, root, driver, plan):
        args, fds = confinement.command(str(root), str(pathlib.Path('/usr/bin/true').resolve()), str(driver), {'PATH': '/usr/bin:/bin', 'LANG': 'C.UTF-8', 'HOME': '/fixture/home'})
        try:
            process = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, pass_fds=fds,
                preexec_fn=lambda: signal.signal(signal.SIGINT, signal.SIG_IGN))
            process.stdin.write(json.dumps(plan).encode())
            process.stdin.close()
            process.stdin = None
            return process
        finally:
            for fd in fds:
                os.close(fd)

    def test_host_ancestors_and_replacement_are_not_visible(self):
        with tempfile.TemporaryDirectory() as name:
            parent = pathlib.Path(name)
            root = parent / 'fixture'
            root.mkdir(mode=0o700)
            (root / 'original').touch()
            (parent / 'TERMINAL.md').write_text('SYNTHETIC_EXTERNAL !`touch /fixture/EXECUTED`')
            (parent / '.git').mkdir()
            (parent / '.atuin').mkdir()
            (parent / '.atuin' / 'TERMINAL.md').write_text('SYNTHETIC_EXTERNAL')
            probe = parent / 'probe.py'
            probe.write_text('import os,pathlib\nassert pathlib.Path("/fixture/original").exists()\nassert not pathlib.Path("/TERMINAL.md").exists()\nassert not pathlib.Path("/fixture/TERMINAL.md").exists()\nassert not pathlib.Path(' + repr(str(parent)) + ').exists()\nprint("CONFINED")\n')
            args, fds = confinement.command(str(root), str(pathlib.Path('/usr/bin/true').resolve()), str(probe), {'PATH': '/usr/bin:/bin'})
            # Replace the named path after obtaining bind FDs, modeling an
            # untrusted writable ancestor race. The pinned inode must win.
            root.rename(parent / 'moved')
            root.mkdir(mode=0o700)
            (root / 'TERMINAL.md').write_text('POISON_REPLACEMENT')
            try:
                result = subprocess.run(args, pass_fds=fds, capture_output=True, timeout=8)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(b'CONFINED', result.stdout)
            finally:
                for fd in fds:
                    os.close(fd)
            self.assertFalse((parent / 'moved' / 'EXECUTED').exists())

    def test_fixture_symlink_and_context_fail_before_launch(self):
        driver = pathlib.Path(__file__).parent / 'e2e' / 'pty_driver.py'
        for kind in ('symlink', 'context'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as name:
                root = pathlib.Path(name)
                if kind == 'symlink':
                    (root / 'escape').symlink_to('/host/private')
                else:
                    (root / 'TERMINAL.md').write_text('!`touch /fixture/EXECUTED`')
                process = self.launch(root, driver, {'root': '/fixture', 'binary': '/atuin', 'endpoint': 'http://127.0.0.1:9', 'steps': []})
                out, err = process.communicate(timeout=8)
                self.assertNotEqual(process.returncode, 0, out)
                self.assertIn(b'denied', err)
                self.assertFalse((root / 'EXECUTED').exists())

    def test_launcher_parent_death_models_global_go_timeout(self):
        with tempfile.TemporaryDirectory() as name:
            root = pathlib.Path(name)
            launcher = pathlib.Path(__file__).parent / 'e2e' / 'confinement.py'
            plan = {'root': name, 'binary': str(pathlib.Path('/usr/bin/true').resolve()),
                    'endpoint': 'http://127.0.0.1:9', 'steps': [], 'test_pause': 'preparation'}
            code = 'import json,subprocess,sys,time; p=subprocess.Popen([sys.executable,"-I",sys.argv[1]],stdin=subprocess.PIPE); p.stdin.write(sys.argv[2].encode()); p.stdin.close(); print("WRAPPER="+str(p.pid),flush=True); time.sleep(60)'
            supervisor = subprocess.Popen([sys.executable, '-I', '-c', code, str(launcher), json.dumps(plan)], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            wrapper_fd = None
            try:
                self.assertTrue(select.select([supervisor.stdout], [], [], 8)[0])
                line = supervisor.stdout.readline()
                self.assertTrue(line.startswith(b'WRAPPER='), line)
                wrapper = int(line.split(b'=')[1])
                wrapper_fd = os.pidfd_open(wrapper)
                self.assertTrue(select.select([supervisor.stdout], [], [], 8)[0])
                self.assertIn(b'PREPARATION_READY', supervisor.stdout.readline())
                owned = self.descendants(wrapper)
                supervisor.kill()  # exactly the launcher-parent death boundary
                supervisor.wait(timeout=5)
                self.assertTrue(select.select([wrapper_fd], [], [], 8)[0], 'die-with-parent guard failed')
                deadline = time.monotonic() + 5
                while any(pathlib.Path(f'/proc/{p}').exists() for p in owned) and time.monotonic() < deadline:
                    time.sleep(0.02)
                self.assertFalse([p for p in owned if pathlib.Path(f'/proc/{p}').exists()])
            finally:
                if wrapper_fd is not None:
                    if not select.select([wrapper_fd], [], [], 0)[0]:
                        signal.pidfd_send_signal(wrapper_fd, signal.SIGKILL)
                    os.close(wrapper_fd)
                if supervisor.poll() is None:
                    supervisor.kill(); supervisor.wait(timeout=5)
                supervisor.stdout.close(); supervisor.stderr.close()

    @staticmethod
    def descendants(pid):
        result = set()
        try:
            children = pathlib.Path(f'/proc/{pid}/task/{pid}/children').read_text().split()
        except FileNotFoundError:
            return result
        for item in children:
            child = int(item)
            result.add(child)
            result.update(Confinement.descendants(child))
        return result

    def test_interrupt_preparation_and_pty_setup_and_hard_parent_death(self):
        driver = pathlib.Path(__file__).parent / 'e2e' / 'pty_driver.py'
        for stage in ('preparation', 'pty_setup'):
            for hard in (False, True):
                with self.subTest(stage=stage, hard=hard), tempfile.TemporaryDirectory() as name:
                    root = pathlib.Path(name)
                    process = self.launch(root, driver, {'root': '/fixture', 'binary': '/atuin', 'endpoint': 'http://127.0.0.1:9', 'steps': [], 'test_pause': stage})
                    try:
                        self.assertTrue(select.select([process.stdout], [], [], 8)[0])
                        self.assertIn(b'READY', process.stdout.readline())
                        owned = self.descendants(process.pid)
                        self.assertGreaterEqual(len(owned), 3)
                        if hard:
                            process.kill()  # PID namespace is independent cleanup guard.
                        else:
                            drivers = [p for p in owned if pathlib.Path(f'/proc/{p}/cmdline').read_bytes().startswith(b'/usr/bin/python3\x00-I\x00/driver.py\x00')]
                            self.assertEqual(len(drivers), 1)
                            os.kill(drivers[0], signal.SIGINT)
                        process.wait(timeout=8)
                        deadline = time.monotonic() + 5
                        while any(pathlib.Path(f'/proc/{p}').exists() for p in owned) and time.monotonic() < deadline:
                            time.sleep(0.02)
                        self.assertFalse([p for p in owned if pathlib.Path(f'/proc/{p}').exists()], 'owned children not reaped')
                    finally:
                        if process.poll() is None:
                            process.kill()
                            process.wait(timeout=5)
                        process.stdout.close()
                        process.stderr.close()


class Watchdog(unittest.TestCase):
    def parent_death_case(self, pending, remove_drain=False):
        with tempfile.TemporaryDirectory() as name:
            root = pathlib.Path(name)
            # Resource existence is represented by independent atomic filesystem
            # entries, NOT concurrent whole-state JSON read/modify/write.
            for resource in ('container', 'unrelated'):
                (root / resource).touch()
            if not pending:
                (root / 'network').touch()
            for channel in ('snapshot-ready', 'snapshot-resume'):
                os.mkfifo(root / channel)
            ready = os.open(root / 'snapshot-ready', os.O_RDWR | os.O_NONBLOCK)
            resume = os.open(root / 'snapshot-resume', os.O_RDWR | os.O_NONBLOCK)
            fake = root / 'docker'
            fake.write_text('''#!/usr/bin/python3
import os,pathlib,sys
r=pathlib.Path(os.environ['ROOT']);a=sys.argv[1:]
assert a[0]=='--host=unix:///var/run/docker.sock';a=a[1:]
def audit(event):
 with (r/'audit').open('a') as f: f.write(event+'\\n')
if a[:2]==['ps','-aq'] or a[:3]==['network','ls','-q']:
 assert 'label=ai.atuin.phase8.run='+os.environ['RUN'] in a
 assert 'label=ai.atuin.phase8.lease='+os.environ['LEASE'] in a
 if a[0]=='ps':
  if (r/'container').exists(): print('a'*64)
 else:
  snapshot=(r/'network').exists()
  if not snapshot and not (r/'snapshot-taken').exists():
   (r/'snapshot-taken').touch();audit('empty-network-snapshot')
   with (r/'snapshot-ready').open('w') as f: f.write('EMPTY\\n')
   with (r/'snapshot-resume').open('r') as f: assert f.readline()=='CONTINUE\\n'
  if snapshot: print('b'*64)
elif a[:2]==['rm','-f']:
 assert a[2:]==['a'*64];(r/'container').unlink();audit('remove-container')
elif a[:2]==['network','rm']:
 assert a[2:]==['b'*64];(r/'network').unlink();audit('remove-network')
else: raise AssertionError('unscoped deletion / global prune')
''')
            fake.chmod(0o755)
            watchdog = pathlib.Path(__file__).parent / 'differential' / 'docker_watchdog.py'
            if remove_drain:
                # Counterfactual in OWNED scratch only: the late-create witness
                # must reject a single-cleanup implementation, never pass merely
                # because the creator happened to run before the first scan.
                original = watchdog.read_text()
                assert original.count('if pending:') == 1
                watchdog = root / 'single-cleanup-watchdog.py'
                watchdog.write_text(original.replace('if pending:', 'if False:'))
            # An independent observer reaps the REAL watchdog and records its
            # numeric OS exit status. pidfd readability alone also accepts a
            # crashed watchdog (the old test silently discarded its stderr).
            wrapper = root / 'run-watchdog.py'
            wrapper.write_text('''import os,pathlib,subprocess,sys
r=pathlib.Path(os.environ['ROOT'])
with (r/'watchdog.stderr').open('w') as err:
 result=subprocess.run(['/usr/bin/python3','-I',os.environ['WATCHDOG'],'watch',os.environ['RUN'],os.environ['LEASE']],stdin=sys.stdin,stderr=err)
(r/'watchdog.exit').write_text(str(result.returncode))
sys.exit(result.returncode)
''')
            env = {'PATH': '/usr/bin:/bin', 'HOME': name, 'ROOT': name,
                'RUN': 'atuin-phase8-unit-fixture', 'LEASE': 'a'*24,
                'ATUIN_PHASE8_DOCKER': str(fake), 'WATCHDOG': str(watchdog.resolve()),
                'WRAPPER': str(wrapper), 'ATUIN_PHASE8_TEST_GRACE': '2',
                'PROTOCOL': 'begin\n' if pending else 'begin\nend\n'}
            supervisor = subprocess.Popen([sys.executable, '-I', '-c',
                'import os,subprocess,time; p=subprocess.Popen(["/usr/bin/python3","-I",os.environ["WRAPPER"]],stdin=subprocess.PIPE,start_new_session=True); p.stdin.write(os.environ["PROTOCOL"].encode()); p.stdin.flush(); print(p.pid,flush=True); time.sleep(60)'],
                env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            pidfd = None
            try:
                self.assertTrue(select.select([supervisor.stdout], [], [], 5)[0])
                observer_pid = int(supervisor.stdout.readline())
                pidfd = os.pidfd_open(observer_pid)
                supervisor.kill()
                supervisor.wait(timeout=5)
                if pending:
                    # The first network listing has ALREADY taken its empty
                    # snapshot but is still in flight. Create after that witness,
                    # then return the old empty result: ONLY a subsequent drain
                    # pass can observe/delete this genuinely late resource.
                    self.assertTrue(select.select([ready], [], [], 5)[0], 'no empty-snapshot witness')
                    self.assertEqual(os.read(ready, 1024), b'EMPTY\n')
                    self.assertFalse((root / 'container').exists())
                    self.assertTrue((root / 'unrelated').exists())
                    (root / 'network').touch()
                    os.write(resume, b'CONTINUE\n')
                self.assertTrue(select.select([pidfd], [], [], 8)[0], 'independent cleanup did not exit')
                diagnostics = (root / 'watchdog.stderr').read_text() + supervisor.stderr.read().decode()
                self.assertEqual((root / 'watchdog.exit').read_text(), '0', diagnostics)
                self.assertEqual(diagnostics, '', diagnostics)
                self.assertFalse((root / 'container').exists())
                self.assertFalse((root / 'network').exists(), 'late-created network survived')
                self.assertTrue((root / 'unrelated').exists())
                audit = (root / 'audit').read_text().splitlines()
                self.assertIn('remove-container', audit)
                self.assertIn('remove-network', audit)
                if pending:
                    self.assertLess(audit.index('empty-network-snapshot'), audit.index('remove-network'))
            finally:
                # Release only our fake helper on assertion failure; never leave
                # it blocked at its rendezvous. No unrelated processes are killed.
                os.write(resume, b'ABORT\n')
                if supervisor.poll() is None:
                    supervisor.kill(); supervisor.wait(timeout=5)
                if pidfd is not None:
                    if not select.select([pidfd], [], [], 8)[0]:
                        # Still-live observer pins this test-owned session ID;
                        # kill ONLY its own watchdog/fake-helper process group.
                        os.killpg(observer_pid, signal.SIGKILL)
                    os.close(pidfd)
                os.close(ready); os.close(resume)
                supervisor.stdout.close(); supervisor.stderr.close()

    def test_partial_create_and_killed_parent_cleanup_only_owned_labels(self):
        self.parent_death_case(pending=True)

    def test_completed_create_and_killed_parent_cleanup_only_owned_labels(self):
        self.parent_death_case(pending=False)

    def test_late_create_witness_rejects_single_cleanup_mutation(self):
        with self.assertRaisesRegex(AssertionError, 'late-created network survived'):
            self.parent_death_case(pending=True, remove_drain=True)


if __name__ == '__main__':
    unittest.main()
