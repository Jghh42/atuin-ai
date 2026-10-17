#!/usr/bin/env python3
"""Fresh pinned original reference build; no source overlays/stale manifests.
Usage: python3 tests/differential/build-oracle.py PINNED_GIT TASK_OWNED_0700_ROOT
Local hashes/labels are consistency checks, not independent producer attestation.
"""
import json
import pathlib
import shutil
import subprocess
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
from build_support import checked_repo, digest, fresh_archive, run_directory, verify_inventory

SERVER = '4d582bc5ceea5b5edfdcf3abb49dc850400cda7c'
CORE = '565877e97ed3ff4840481b8d47e12eb2f9854039'


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    repo, root = [pathlib.Path(p).absolute() for p in sys.argv[1:]]
    work = run_directory(root)
    checked_repo(repo, SERVER)
    if shutil.disk_usage(root).free < 3 * 1024**3:
        raise RuntimeError('need >=3GiB free before oracle build')
    context = work / 'reference'
    archive_hash, expected = fresh_archive(repo, SERVER, context)
    dockerfile = pathlib.Path(__file__).with_name('Dockerfile.oracle').resolve()
    recipe_hash = digest(dockerfile)
    iid = work / 'image-id'
    verify_inventory(context, expected)
    docker_config=work / 'docker-config'
    docker_config.mkdir(mode=0o700)
    env={'PATH':'/usr/bin:/bin','HOME':str(work),'DOCKER_CONFIG':str(docker_config)}
    subprocess.run(['/usr/bin/docker', '--host=unix:///var/run/docker.sock', 'build', '--platform=linux/amd64', '--iidfile', str(iid),
        '--label', 'ai.atuin.source.archive=' + archive_hash,
        '--label', 'ai.atuin.build.recipe=' + recipe_hash,
        '-f', str(dockerfile), str(context)], env=env, check=True, timeout=1800)
    verify_inventory(context, expected)
    checked_repo(repo, SERVER)
    manifest = dict(server_sha=SERVER, core_sha=CORE, image_id=iid.read_text().strip(),
        source_archive_sha256=archive_hash, dockerfile_sha256=recipe_hash,
        builder_sha256=digest(pathlib.Path(__file__)),
        support_sha256=digest(pathlib.Path(__file__).resolve().parents[1] / 'build_support.py'),
        fresh_inputs=True, source_worktree_unchanged=True)
    (root / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print('ATUIN_ORACLE_MANIFEST=' + str(root / 'manifest.json'))


if __name__ == '__main__':
    main()
