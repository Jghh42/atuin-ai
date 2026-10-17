"""Opt-in REAL consumer regressions on a disposable clean-tagged fixture.

No producer or Git command is invoked here. Supply a private fixture built from
known public source with its own synthetic Git commit/test tag, never main or a
published artifact. The explicit marker below belongs inside that fixture's .git.
"""
import argparse
import hashlib
import json
import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
from run import REPO, verify_resource_artifacts

TAG = 'v0.0.0-test.resource-release'
MARKER = 'disposable resource mode fixture; synthetic Git identity; no remotes\n'


class RealReleaseAcceptance(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='resource-mode-assets-')
        self.addCleanup(self.temporary.cleanup)
        self.artifacts = pathlib.Path(self.temporary.name)/'assets'
        shutil.copytree(ARTIFACTS,self.artifacts)
        self.artifacts.chmod(0o700)

    def verify(self, release=True):
        return verify_resource_artifacts(self.artifacts,SOURCE,release=release)

    def rehash_manifest(self, change):
        path = self.artifacts/'manifest.json'
        data = json.loads(path.read_text())
        change(data)
        path.write_text(json.dumps(data,sort_keys=True,indent=2)+'\n')
        sums = self.artifacts/'SHA256SUMS'
        sums.write_text(''.join(hashlib.sha256((self.artifacts/name).read_bytes()).hexdigest()
                               +'  '+name+'\n' for name in sorted(
                                   p.name for p in self.artifacts.iterdir() if p.name != 'SHA256SUMS')))

    def test_real_clean_tagged_explicit_release(self):
        manifest = self.verify()
        self.assertEqual(manifest['mode'],'CLEAN TAGGED; publication requires separate approval')
        self.assertFalse(manifest['source']['dirty'])
        self.assertEqual(manifest['version'],TAG)

    def test_real_release_without_optin_refuses(self):
        with self.assertRaisesRegex(AssertionError,'resource evidence requires DEVELOPMENT'):
            self.verify(release=False)

    def test_real_missing_private_fixture_tag_refuses(self):
        # Only this disposable fixture's private test ref, restored byte-for-byte.
        ref = SOURCE/'.git'/'refs'/'tags'/TAG
        data = ref.read_bytes()
        ref.unlink()
        try:
            with self.assertRaises(subprocess.CalledProcessError):
                self.verify()
        finally:
            ref.write_bytes(data)

    def test_real_dirtied_fixture_source_refuses(self):
        path = SOURCE/'README.md'
        original = path.read_bytes()
        path.write_bytes(original+b'\nsynthetic private fixture mutation\n')
        try:
            with self.assertRaisesRegex(AssertionError,'source inventory/identity misrepresentation'):
                self.verify()
        finally:
            path.write_bytes(original)

    def test_rehashed_unknown_mode_refuses(self):
        self.rehash_manifest(lambda m:m.update(mode='unknown'))
        with self.assertRaisesRegex(AssertionError,'mode'):
            self.verify()

    def test_rehashed_source_identity_tamper_refuses(self):
        self.rehash_manifest(lambda m:m['source'].update(inventory_sha256='0'*64))
        with self.assertRaisesRegex(AssertionError,'source inventory/identity misrepresentation'):
            self.verify()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source',type=pathlib.Path,required=True)
    parser.add_argument('--artifacts',type=pathlib.Path,required=True)
    args = parser.parse_args()
    SOURCE = args.source.resolve(strict=True)
    ARTIFACTS = args.artifacts.resolve(strict=True)
    assert SOURCE != REPO.resolve(strict=True),'never mutate the working repository'
    assert (SOURCE/'.git'/'resource-mode-test-fixture').read_text() == MARKER
    assert (SOURCE/'.git'/'refs'/'tags'/TAG).is_file(),'private unpacked test tag required'
    assert json.loads((ARTIFACTS/'manifest.json').read_text())['version'] == TAG
    # Establish the real full independent contract before mutating any fixture.
    verify_resource_artifacts(ARTIFACTS,SOURCE,release=True)
    unittest.main(argv=[__file__])
