# Phase 8 test-only tooling and artifact provenance

No new Go module dependency, runtime backend setting, credential import or
production listener option was added. Python's stdlib `pty`, `termios`, `fcntl`,
`select`, `subprocess` and JSON/HTTP modules drive the actual client; no copied
PTY library is vendored. Python is external test tooling (PSF license), not
linked/distributed with the Go server. Rust/Cargo (MIT/Apache-2.0), Elixir
(Apache-2.0), Erlang/OTP (Apache-2.0), Gleam (Apache-2.0), Docker/Moby
(Apache-2.0), and base distributions/dependencies remain external build/oracle
tooling, with their licenses/notices retained in their distributions/images.
No binary/image/toolchain is committed or packaged as a deployment artifact.
Existing Atuin MIT and reference Apache license texts remain in `LICENSES/`.
Per-case derivation and the adapted reference Dockerfile are attributed in
[`phase8-manifest.json`](../../testdata/compat/phase8-manifest.json) and `NOTICE`.

## Corrected fresh client build, Linux amd64, 2026-10-16

The initial cached-input build and HOME/XDG-only runs were NOT review-cleared:
installed marker files, source overlays and ancestor discovery were insufficient
proofs. Historical artifact `34e01712...` and its logs are retained, not reused
as the corrected gate. Old runs cannot establish absence of ancestor context
reads/execution; no incident is inferred and no private ancestor content was
inspected. Corrected local manifests verify consistency, NOT independent
producer attestation or adversarial same-UID producer trust.

- Source: Atuin `4ca8b77c8a526b8c294eb864a2b74c95c685c983`, MIT.
- Original checkout HEAD and clean status verified before/after. A full
  `git archive` materializes sparse-excluded tracked files, including
  `vendor/axoasset`; no source patch, feature removal or CLI replacement.
- Fresh archive extraction into a NEW private per-build directory; COMPLETE
  source inventory (including added/removed files, directories, executable modes
  and symlink targets) checked before/after compile. No retained Cargo HOME,
  compiler wrapper/config ancestor, target artifact or installed toolchain is
  reused. Only official component DOWNLOAD archives may be cached; their bytes
  are checksum-verified on EVERY use. Empty installed markers are ignored.
  Toolchains are freshly installed and their full installed inventory checked
  unchanged after compilation. `Cargo.lock` SHA256:
  `83997196519a5da77b5f126a4f479b920184aefb4bdc80a8754ef775d09de74d`.
- Source archive SHA256:
  `eec76d918575cfa6e22bdeae3492c7db11c06d1c208e285ac408bf67bd39d709`.
- Exact command: `cargo build --locked --release -p atuin`, default features
  (including AI/daemon/PTY proxy), no changed API/source/features.
- Overrides: jobs=2, incremental=0, release debug=0, strip=symbols, LTO=false.
  No global install, sudo or rustup modification. Isolated HOME/CARGO_HOME,
  toolchain and target directories under task-owned scratch.
- Official Rust **1.98.0** (`88d9e12ae178fab0fb5cc050a94da85685d449ea`,
  LLVM 22.1.8), matching `rust-toolchain.toml`. Official component archives
  from `https://static.rust-lang.org/dist/2026-08-20/`, checksums pinned from
  the official 1.98.0 channel manifest:

| x86_64-unknown-linux-gnu component | SHA256 |
| --- | --- |
| rustc | `0e37cb339f447fc44d6d781073bacacebfdc5612f2600e4c7e84c266f5f3aced` |
| cargo | `2f512d170d3dd23e16ababcda32ee2e6d5172d861a7af1f504e0b1e270cafab9` |
| rust-std | `f5022e6c95a5ad23cca2513dc8281200f585fa188de6370aa37b128a43f876a3` |

- Resulting local Atuin binary SHA256:
  `b940cd03e687cb4a8e6d227c57d1ca17101308b52efb65a1ef211bc8e2d0ac11`.
  This identifies the tested build, not a universal reproducibility claim.
- Latest narrow-followup COLD build: **11m06s** (the prior corrected 10m53s
  build is historical); fixed linker/compiler paths `/usr/bin/gcc`
  and `/usr/bin/g++` are recorded in its manifest, with no inherited wrappers.
  Host GCC is Ubuntu **15.2.0-16ubuntu1**, canonical executable
  `/usr/bin/x86_64-linux-gnu-gcc-15`, SHA256
  `b5f1b773a7c733738352000c92a077dc5852a1a2fc6d836b1e411be1e9ec5f88`.
  Cargo runs in a filesystem/PID namespace exposing only fresh `/work`, verified
  toolchain, system `/usr` and public TLS/DNS files (NOT /etc/ssl/private).
  Build recipe SHA256 `52a7e17c575a19c29b3730b527e8aaf0461f251b1b82c9d83654189531250cdf`;
  shared integrity helper SHA256
  `dcc2bf6e919e6156006fc9b0adcf5cd0df1dd26ec0f5b6d957bd79604ad2de75`.
- Historical initial build: **11m05s**, sampled combined active rustc RSS approximately
  2.8 GiB, task scratch approximately 2.6 GiB. Repository filesystem's lowest sampled free
  space was 6.8 GiB during provisioning (initial 9.4 GiB); `/tmp`
  remained separate tmpfs. These are build observations, NOT Phase 9 server
  resource gates. Script bounds builds to 30 minutes and stops below 1 GiB free.

Rebuild script: `tests/e2e/build-atuin.py`. Its local JSON manifest binds source,
flags/toolchain and artifact hash; it is NOT a cryptographically signed build
attestation. Before each build the prior success manifest is invalidated. E2E
requires fresh-input/unchanged-source flags, archive/lock/component/flags/command
pins, current recipe/helper hashes and the regular-file binary hash. These are
local consistency checks, NOT independent producer attestation.
Scratch artifacts are retained for independent review, not installed globally.

## Original reference source container (oracle only)

Server `4d582bc5ceea5b5edfdcf3abb49dc850400cda7c` and core
`565877e97ed3ff4840481b8d47e12eb2f9854039`, both Apache-2.0. Server archive
SHA256 `c9424d674e1a237b7f3332f95e7fd26a048e27d72027f0e8f3b87f8464382b95`.
`mix.lock` selects exactly that core; the build asserts its clean HEAD after
compilation. A NEW archive context is used every build, never overlaid on prior
`lib/` files; its complete inventory is checked before/after Docker build.
Original server `lib/`, `config/`, `mix.exs` and `mix.lock` hashes are checked
before/after dependency fetch/compile/release. A poisoned leftover `.ex` file
regression rejects overlays and additions. Manifests are invalidated before
build and bind archive, current recipe/helper hashes and fresh/unchanged flags.
The checkout/application sources remain unmodified.

`tests/differential/Dockerfile.oracle` adapts the original build recipe only:

- Builder: official HexPM Elixir 1.18.3 / OTP 27.3.4.13,
  `sha256:6ad4d47ccde7f947adf598ef57acec9381df7fb4d73f996a0442f49be6615590`.
- Runner: official Debian bullseye-20260610-slim,
  `sha256:a975c02d3f80b0ee78991b9191209709efb6f641c0d9bebe18c443a4d3de32c9`.
- Apt uses signed official Debian/debian-security snapshot `20260610T000000Z`;
  only historical Release expiry is disabled, NOT package signature checking.
  This replaced mutable security-pool URLs that actually failed with 404s.
- Gleam 1.16.0 official GitHub release amd64-musl archive SHA256:
  `1ca1183719a32c2cfb15a845ff6192bd2cb4eaf4d8913bfad9662349f8d9a7ff`,
  verified against release asset metadata before extraction (no script pipe).
- Hex **2.5.3** archive (Elixir 1.16.0 build) and Rebar3 **3.22.0**
  (Elixir 1.15.0-rc.0 build) are pinned with official `repo.hex.pm/installs/`
  CSV SHA512 checksums, verified during installation; no mutable `local.hex`
  or `local.rebar` latest-version selection. Both tools are Apache-2.0.
  Exact URLs/digests are in the Dockerfile.
- Erlang threads bounded with `+S 2:2 +A 2`; 30-minute build budget.
- Final local source-built image ID:
  `sha256:74e20df94de3e6fcd31b0d0a53026fd5dde251934acc688da87058355b49af61`.
  Dockerfile SHA256:
  `52e764c4c335f7f069a56be214d7d277cc9f9ec117b4391a9ee2e379bfc6624c`.
- Existing `ghcr.io/atuinsh/atuin-ai-server:latest` was inspected but NOT used
  to claim the source-pinned differential gate: its source equivalence was not
  proven. The actual suite uses the source-built image and verifies labels/ID.

Fake Chat Completions sidecar runs a static local Go test helper on Alpine 3.22,
`sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce`.
Oracle and fake backend share a disposable **internal Docker bridge with NO
published ports**. A destination-restricted helper inside that network sends
identical fixture bytes to the oracle and returns structured HTTP results.
Native Go serves Atuin only on 127.0.0.1. No host-network container, external
API exposure or real key is used. Oracle: 256 MiB/2 CPUs/128 PIDs; fake: 64 MiB/
1 CPU/32 PIDs. Unique owned names and run/lease labels are established BEFORE
creation. An independent stdin-EOF watchdog survives Go failure/timeout, removes
ONLY exact labeled containers/networks, and drains uncertain late creates for a
bounded 30s window. A workflow `always()` cleanup additionally scopes to its
unique run label. No global prune or deletion of unlabeled names/images/caches.
A synthetic partial/late-create + killed-parent regression preserves unrelated
resources. An unresponsive Docker daemon is an explicit cleanup/gate failure,
not an unlimited cleanup guarantee.

Host test client is existing `/usr/bin/docker`, package `docker-ce-cli`
`5:29.7.2-1~ubuntu.26.04~resolute`, Docker CE 29.7.2, commit `a7dcaa6`, API 1.55,
SHA256 `dbf7fd0c0ae54d208314ee5c19a97a12d966dab039b7d94872ca91cbe490373c`.
All builders/test helpers/watchdogs use explicit
`--host=unix:///var/run/docker.sock` with isolated HOME/DOCKER_CONFIG, never a
normal user's Docker auth/context store. Current oracle builder SHA256:
`31a3c91b378ca87ee34789abf8f33afb366df851d36c9eb9a5343b02da5dc1fd`.
Daemon also 29.7.2. Docker is only build/test infrastructure, not a runtime
server dependency. No extra Docker client binary was downloaded.

## Filesystem/PID confinement prerequisite

Every actual Atuin preparation/PTY/daemon/tool process now runs INSIDE a single
Bubblewrap namespace: new user/PID/IPC/UTS namespaces, same loopback network,
fd-pinned private `/fixture`, read-only executable/system `/usr`, new `/proc` and
`/dev`, synthetic passwd/group/os-release/nsswitch files, and a read-only virtual
root. Host HOME, /tmp, ancestors, projects and private stores are NOT mounted.
Fixture symlink/context/project metadata fails closed before each child launch.
No repeated metadata check under host world-writable /tmp is relied on.

External existing `/usr/bin/bwrap`: Bubblewrap **0.11.1**, distro package
`0.11.1-1ubuntu0.3`, SHA256
`523da3e7399044be5163aee6f57a77a6bef7454376e28f0a0627920bae1b76b6`.
Source <https://github.com/containers/bubblewrap/>, LGPL-2.0-or-later, copyright
Alexander Larsson, Simon McVittie, Collabora Ltd.; distro copyright/license text
remains in `/usr/share/doc/bubblewrap/copyright` and system common licenses.
No Bubblewrap implementation/binary is vendored, linked or shipped with the
server. It is required external Linux test tooling, not a deployment feature.
CI requires that version plus usable unprivileged namespaces; unavailable or
restricted runner prerequisites FAIL the required gate rather than skip green.
No DEVELOPER-host sysctl, sudo/global installation or branch-protection setting
is changed. Final independent narrow review CLEARED the hosted-CI prerequisite
setup below; code review does not establish an actual hosted-kernel/CI pass.

Corrected artifact manifests:
`/home/pi/.cache/atuin-ai-phase8-fixed-cli/manifest.json` and
`/home/pi/.cache/atuin-ai-phase8-fixed-oracle/manifest.json`.
Original artifacts are retained as historical only; new hard gates reject their
incomplete proof fields. No corrected CI run is claimed until actually executed.

## Narrow-review CI provisioning addition (2026-10-17)

The compatibility job no longer assumes an ubuntu-latest packaged version or
usable namespaces. It selects `ubuntu-24.04`, installs bounded prerequisites from
that VM's signed Ubuntu repositories (8-minute step, 180/240s apt commands,
zero acquisition retries, bounded lock wait, no recommended packages), then
builds the OFFICIAL release into a new private prefix with two compile jobs.
Package versions are saved to `bwrap-dependencies.txt`; compiler/Meson/Ninja/
pkg-config paths, executable hashes and versions are saved in its manifest.
This is ephemeral CI setup, NOT developer-host/global product installation.

- Source: <https://github.com/containers/bubblewrap/releases/tag/v0.11.1>
- Exact archive: `https://github.com/containers/bubblewrap/releases/download/v0.11.1/bubblewrap-0.11.1.tar.xz`
- SHA256 `c1b7455a1283b1295879a46d5f001dfd088c0bb0f238abb5e128b3583a246f71`
  (official release asset digest, retrieved from GitHub release metadata and
  independently matched against downloaded bytes, not an installed marker).
- No source patches. Fresh Meson build, release mode, `--wrap-mode=nodownload`,
  `require_userns=true`; man pages/completions/upstream test-build targets are
  omitted as packaging/build-only extras. Runtime namespace/PID/filesystem
  isolation is NOT disabled. SELinux support retains upstream auto detection.
  Non-setuid/setgid executable required. Complete source inventory verified
  unchanged; complete installed file/mode/symlink/hash inventory checked on
  EVERY configured-tool selection. Actual executable `--version` must match.
- LGPL-2.0-or-later copyright notices remain in source; full upstream `COPYING`
  and `README.md` retained under `prefix/share/licenses/bubblewrap`. No source
  implementation or compiled binary is vendored/shipped/linked with Go server.
- `ATUIN_TEST_BWRAP_MANIFEST` is test-only. `bwrap_tool.py` verifies the pinned
  release, current builder/selector/support hashes, complete prefix inventory,
  owned/private root, regular non-setuid executable and actual version. Invalid
  configured metadata NEVER falls back to a system binary. No variable keeps
  the independently reviewed `/usr/bin/bwrap` path unchanged locally.
- CLI harness edits are ONLY that explicit verified program selection and
  whitelisting the manifest pointer into its existing sanitized Python launcher
  environment; actual namespace arguments, PTY/tool/permission/lifecycle code
  remain unchanged. Atuin builder selects the same verified program. This recipe
  edit requires a real fresh Atuin build, not a stale-manifest SHA rewrite.

### Hosted job AppArmor prerequisite (privileged path NOT yet executed)

Ubuntu's unprivileged-userns AppArmor restriction can reject a newly built
binary outside the distro profile, even when `/usr/bin/bwrap` works. When the
kernel restriction is 1, `ci_bwrap_profile.py` loads a new, exact executable-path
profile named ONLY for that numeric run/attempt, granting `userns` to the verified
non-setuid Bubblewrap. No broad wildcard, root execution of bwrap, global sysctl,
AppArmor disable, sandbox bypass or privileged host mount. The helper refuses
non-GitHub-hosted/developer use, constrains paths to RUNNER_TEMP, records ownership
BEFORE load, and an `always()` step removes ONLY that exact named profile.
No `/etc/apparmor.d` profile is installed. No branch-protection/PR metadata is
modified; administrators still must require the aggregate externally.

Final independent narrow review CLEARED this explicit privileged setup; that
is not a claim of CI success. A mandatory unprivileged same-argument user/PID/FD/filesystem/root-
readonly namespace probe must pass AFTER setup and before all actual CLI gates.
Failure remains red; no skip/no-sandbox/relaxed-confinement fallback is present.
The kernel profile load/unload has NOT run locally or on CI. Mocked tests prove
scope/partial-failure cleanup behavior; real parser `-Q -T` validates syntax
without loading ANY kernel profile. An unresponsive/refusing runner stays blocked.

### Local source-build evidence and failures

Developer host lacked Meson/Ninja/libcap headers. Four signed-index Ubuntu
packages were downloaded/extracted into OWNED scratch only (no apt install):

| Package | Version | Downloaded .deb SHA256 |
|---|---|---|
| meson | 1.10.1-1ubuntu2 | `854f70286df0fa259e9e6c2c18cdfeb41276dc3afa4356f2a2605439baf31d5f` |
| ninja-build | 1.13.2-1 | `533239a464183f041f7c979f899fd8419e31ac82366540e57902a255de78f156` |
| libcap-dev | 1:2.75-10ubuntu2 | `74fa71aa089d90d720722f6278314b90b72fa2e1631aea5dc32b770d0324ce86` |
| libcap2 | 1:2.75-10ubuntu2 | `150603a0792b1d22aa0bde0a26c27b413018c799284003e4b43cba587a5b3a18` |

OS license texts retained under extracted `usr/share/doc`; these are external
build tooling, not new Go/server dependencies. An owned Python wrapper points
Meson at its extracted dist-packages; an owned-stage `usr/lib64` symlink resolves
its libcap .pc's multiarch mismatch (build1 failure retained). Build2 found JSON
list/tuple inventory comparison in the NEW selector; canonical comparison plus a
serialized positive regression fixed it. Fresh build3 succeeded:
`/home/pi/.cache/atuin-ai-phase8-bwrap-built3/manifest.json`, binary SHA256
`901419f803d7d5ed8957652e43608cf1841cbf72d977ddd4427545422ef6636c`.

The source prefix's namespace probe then FAILED (`setting up uid map: Permission
denied`), consistent with this developer host's per-path AppArmor restriction.
The synthetic prefix-only confinement regressions likewise failed at namespace
creation, never inside actual Atuin. No developer-host policy was changed to
force passes; those failures are preserved. The reviewed SYSTEM bwrap remains
the local CLI build/test prerequisite. The new CI exact-prefix path is not a
kernel-verified CI pass until an actual hosted run after authorized delivery.
