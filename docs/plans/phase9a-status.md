# Phase 9a — packaging / installer review checkpoint

## Exact-HEAD delivery receipt — supersedes historical PENDING language

Delivered CLEAN main `edfa90452922cc2792e6e001277821bfcad6da3d`.
Coordinator verified ALL exact-HEAD hosted workflows GREEN:
[packaging39004040516](https://github.com/Jghh42/atuin-ai/actions/runs/39004040516),
[Go39004040656](https://github.com/Jghh42/atuin-ai/actions/runs/39004040656), and
[compatibility/aggregate39004040576](https://github.com/Jghh42/atuin-ai/actions/runs/39004040576).
The user has the DEVELOPMENT Mac artifact/hash/checklist and volunteered native
execution; outcomes remain PENDING, not inferred from CI/crossbuild. Phase9b was
then separately authorized; [resource review checkpoint](phase9b-status.md).
No release/tag/public publishing is authorized. Production live/catalog and
native Mac release gates remain NOT PASSED/PENDING.

## Final independent acceptance / authorized code delivery

Coordinator reported BOTH focused independent reviews **CLEARED** all blocking
findings and warnings: installer curl-q/min8.4/real TLS/all three caps and Darwin
negatives; Unicode15/17 full grants/pins; hidden-index/sparse/full-HEAD source
truth; and the independent exact artifact consumer/mutation verifier.
Independent corrected verification **PASSED**: 36 tests, 32 required artifact
mutations, make-check/build/diff/ShellCheck/formatting, BOTH minimum targets from
TWO different absolute source/build-cache paths, all 24 legal entries/full
inventories, actual installer and relocated Linux 503/lock/signals/restart.
This clears the **9a code and local packaging gates**, not hosted CI or release.

The verified development input was the immutable 212-file source inventory
`14262c874f3093b066968a6e43153a7cc9777dfd4fb69c7b963608311cc7edbd`.
Its reviewed artifacts at
`/home/pi/code/atuin-ai/dist/phase9a-review-fixed/` remain valid for THOSE prior
inputs, with honest base revision/dirty=true, never relabeled as a clean commit.
This documentation receipt changes the holistic source inventory; the delivery
commit also changes revision identity. Do not claim those prior artifacts match
the new commit. Prefer fresh exact-commit CI artifacts for the user's native
handoff after ALL exact-HEAD workflows pass. CI versions use
`v0.0.0-dev.ci.SHA`, artifact name `development-packaging-SHA-ATTEMPT`; these are
DEVELOPMENT workflow artifacts only, not tags or public releases.

Normal focused commit/push to main is now authorized. Actual hosted 9a packaging
CI, Go Checks and existing 20-CLI/14-oracle compatibility/aggregate are **PENDING**
until demonstrated for that exact pushed SHA. The root coordinator owns that
ALL-workflow checkpoint; Phase9b must not start on old-SHA green or partial CI.
Native Mac execution remains **PENDING user testing**, and production live
login/lifecycle/eligible catalog remain **NOT PASSED**. Resource/load/leak gates
are Phase9b **NOT IMPLEMENTED**; public-release approval is NOT granted.

Nonblocking review follow-up: consider explicit bounded joining of the real-TLS
fixture's daemon handler threads in a later reviewed test-hardening change.
This suggestion did not block acceptance; no implementation is changed after
verification in this delivery. Existing finite client guards, scoped cleanup and
unexpected-server-error failures remain as reviewed.

## Historical review-fix checkpoint (superseded by acceptance above)

The original `e76a85daa482a305c1229ef98f74e2d4a94a5d85e4af4f50028d32b38c739f5f`
source's 27 tests/full checks/two-path builds/shipping smoke were independently
reproduced, but that was code-under-test coverage, **NOT clearance of the review
defects**. Those artifacts and the prior handoff are preserved and SUPERSEDED;
no binary has been sent to the user. Review found and this sole-writer follow-up
fixes these blockers/warnings before a fresh exact-source handoff:

1. All installer curl calls (including version probe) and outer bootstrap now
   put `-q` FIRST, disabling ambient `.curlrc` insecure/auth/netrc/output settings.
   Require **curl >=8.4.0 before fetch**, with actionable failure on old curl:
   before8.4, max-filesize cannot cap unknown-length bodies. The explicit minimum
   is chosen instead of a fragile POSIX FIFO/head/process-status pipeline; older
   bundled curl can be replaced on PATH using the user's trusted package manager,
   never by this installer. Manual verified binary/archive use remains possible.
2. Real curl TLS/redirect/chunked tests now use ephemeral certificates/private
   synthetic HOME/rc/netrc and forced loopback only. A no-q control proves the
   synthetic rc would actually disable trust/send auth/write output. Corrected
   requests must ignore it; untrusted CA fails with curl60, HTTP redirect with
   curl1, positive HTTPS/chunked succeeds, and unknown-length checksums/binary/
   legal overflows each stop with curl63 while scratch stays <=65536/67108864/
   1048576 bytes. The original stub redirect/oversize tests did NOT prove this.
   Added checksummed wrong-format/CPU Darwin negatives and old-curl rejection.
3. [Unicode15 and Unicode17 grants/attribution](../provenance/unicode-data.md)
   are now legally complete for all six uniseg tables and four ACTUALLY selected
   Go1.27.2 stdlib/vendor tables. Full grants come from immutable authoritative
   Unicode-org sources, not guessed latest copyright years. Explicit pinned
   legal/table selection/version/hash gates cover archives AND installer legal.
4. Hidden assume-unchanged/skip-worktree and sparse checkout/index states are
   rejected for development too. Full HEAD tree blobs/modes (including missing
   files) are compared to actual normalized snapshot bytes, not inferred from
   status; hidden core.filemode=false mutations are honestly dirty. Release
   cannot label a mismatched snapshot CLEAN. Dedicated regressions exercise all.
5. A separate consumer verifier imports NO producer inventory implementation:
   exact eight-file/seven-checksum/six-nonrecursive-manifest-asset contract, both
   minimum targets, file types/modes/bytes/sizes, independent actual source/HEAD
   truth, exact reviewed legal bytes, archive types/modes/normalization and the
   complete installer legal bundle. Thirty-two mandatory real-artifact mutations
   reject missing Darwin checksum, extra/missing/duplicate output/rows, wrong
   manifest sizes/assets/legal content/modes and source inventory lies, even
   after producer-style outer rehashing. CI's required artifact step runs them.

Follow-up writer validation so far: **36 default tests** (including five real
curl/TLS fixture tests) and **31 initial artifact mutations + actual Linux
shipping smoke PASS** at `/tmp/phase9a-reviewfix-artifact-first.log`, using
`/home/pi/code/atuin-ai/dist/phase9a-fix-validation/`. Both-target/two-path
packaging PASS: `/tmp/phase9a-reviewfix-package-validation.log`. A further
self-consistently rehashed BOTH-archive/legal-bundle corruption expands the
required final suite to 32 mutations. These earlier results are a validation
snapshot, not the final exact-source bundle. Final fresh-source
make-check/build/diff/bundle/smoke receipts and hashes are supplied outside the
source inventory by the coordinator handoff. Independent focused re-review is
PENDING; no commit/push/tag/release/user artifact is authorized.

Preserved new first failures: `/tmp/phase9a-reviewfix-focused-first.log` exposed
literal escaped newline/CRLF bytes in NEW test rc/chunk fixtures (not curl or
runtime failure). Correct bytes fixed the fixture, without padding its 10s guard;
owned test process groups now clean on timeout. Expected TLS client rejection
peer-aborts are explicitly classified; all other fixture server errors FAIL.
`/tmp/phase9a-reviewfix-legal-preflight.log` failed the NEW Unicode version regex
because it was overescaped; inspected actual Version17 constants and fixed the
regex, not the completeness check. Failed output marker remains under
`dist/phase9a-fix-legal-preflight/FAILED`. Production code/state is unchanged.

## Historical initial implementation checkpoint (SUPERSEDED)

Started from CLEAN main `a50af9a16ca08e9898077ad52a0b2551d5fa0a56`.
Coordinator supplied BOTH exact-HEAD Phase 8 green receipts:
[compatibility 38999649689](https://github.com/Jghh42/atuin-ai/actions/runs/38999649689)
(20 actual CLI / 14 oracle, verified source-prefix Bubblewrap/AppArmor/kernel
probe, owned cleanup and aggregate) and
[Go checks 38999649761](https://github.com/Jghh42/atuin-ai/actions/runs/38999649761)
(make check/build). Earlier CI failures remain in the
[Phase 8 receipt](phase8-ci-receipt.md); they were diagnosed, not blindly rerun.

## Scope / gates

Phase 9a only: minimal no-state version dispatch; deterministic two-target,
two-absolute-path packager; raw-binary plus verified legal-text POSIX installer;
offline adversarial tests; artifact-upload-only read-permission CI; operations,
provenance and native Mac handoff. [Detailed contract](../packaging.md).
No production OAuth/Responses/discovery/config/default port/model change,
no private/probe credentials, real-provider calls or resource/profiling work.
No tag/commit/push/public release is authorized at this checkpoint. Independent
read-only review and verification must precede delivery/native artifact handoff.

Minimum Linux amd64 CGO0/GOAMD64=v1 and Darwin arm64 CGO0 binaries are cross-built
from Linux with exact Go1.27.2. Stable linker identity records actual dirty source
without pretending a clean release. Development-only `v0.0.0-dev.phase9a` is a
build label, not a created/published Git tag. All outputs live under ignored,
owned `dist/` directories; final full paths/hashes are returned outside the source
snapshot to avoid self-referential inventory claims. Two-path consistency is NOT
independent attestation. Clean release mode requires a reviewed clean source and
an already-existing exact-HEAD tag; no workflow/script publishes or creates one.

Installer design deliberately uses raw executables + checked legal bundle, not
archive extraction. Exact-tag downloads never re-resolve latest; normal HTTPS
and bounded redirects/sizes/timeouts are mandatory. Checksums are same-source
integrity, not publisher authentication. Destination symlinks/control paths and
wrong platform/architecture fail closed. Both files stage in the destination
filesystem; legal commits first and binary commits in one atomic rename. Old
binary survives precommit failures/catchable interruptions; a version-specific
legal text may remain after failed binary rename. SIGKILL/power loss can retain
owned staging files and is not fsync durability. No config/credential/profile/
autostart/sudo/Gatekeeper change. New config permissions (0700 directories,
0600 files) are documented only; no new loader-enforcement behavior.

## Preserved first failures / fixes

- `/tmp/phase9a-package-first.log`: legal verification correctly failed because
  uniseg's actual module license path is `LICENSE.txt`, not `LICENSE`. Corrected
  the exact upstream path; no license content/check was weakened.
- `/tmp/phase9a-tests-first.log`: newline destination path bypassed a POSIX grep
  character-class check (newline is the line delimiter). Added explicit newline
  rejection; version also rejects any non-safe alphabet before its strict regex.
  The following subcase inherited that synthetic write and consequently failed
  the private-state sentinel; no real user state was involved.
- `/tmp/phase9a-shipping-first.log`: smoke expected a nested error object, while
  the actual specified API uses flat `error`/`message`. Read runtime serializer,
  corrected the shape, then `/tmp/phase9a-shipping-validation.log` exposed that
  the model-list route uses `models_unavailable` (not chat's
  `upstream_unavailable`). Read the actual route and assert its COMPLETE existing
  error/message object. No assertion was dropped and no production runtime changed.

## Writer observations so far

- Version unit tests and initial crossbuild/two-path packaging passed.
- `/home/pi/code/atuin-ai/dist/phase9a-early/` is an EARLY developmental snapshot,
  **not the final reviewed handoff**. No unreviewed binary was sent to the user.
- `/home/pi/code/atuin-ai/dist/phase9a-notices/` demonstrated two-path builds with
  Go source notices; it predates final installer/legal/test refinements and must
  NOT be mistaken for the final checkpoint. Failed output marker is retained at
  `/home/pi/code/atuin-ai/dist/phase9a-first/FAILED`.
- Corrected initial offline tests: **23 passed** in
  `/tmp/phase9a-tests-fixed.log`. Historical pre-review suite: **27 tests passed** in
  `/tmp/phase9a-adversarial-final-prebuild.log`, including synthetic Darwin
  aliases, shasum fallback, old-byte/mode preservation and a SUCCESS-returning
  child interrupted by SIGINT/SIGHUP/SIGTERM (only the trap prevents commit).
  POSIX syntax, local ShellCheck and diff checks pass.
- Full `make check` / `make build`: **PASS** in
  `/tmp/phase9a-final-make-check.log` and `/tmp/phase9a-final-make-build.log`;
  includes Go unit/race/vet/Staticcheck/nilness/govulncheck (no vulnerabilities),
  all four unchanged five-second fuzz targets and 27 offline Python tests.
  Default external CLI/oracle skips do not count as new compatibility runs.
- `/home/pi/code/atuin-ai/dist/phase9a-validation/`: BOTH targets, identical
  same-target binaries/normalized archives/legal/build metadata across two
  fresh absolute paths and build caches **PASS**, with hostile inherited
  GOFLAGS/GOWORK/GOENV/CGO_ENABLED/GOEXPERIMENT/GOTOOLCHAIN ignored by the
  allowlisted build environment. `/tmp/phase9a-package-validation.log`.
- Complete checksum/archive/license/source inventories and relocated Linux
  shipping executable version/no-state/complete missing-credential models 503/
  lifetime-lock/SIGINT/SIGTERM/restart **PASS**. Actual raw-asset installation
  through fake HTTPS plus execution of the installed version command **PASS**:
  `/tmp/phase9a-actual-install-smoke.log`. No release network or live auth.
- These named development outputs are validation snapshots, not the final
  independent-review handoff. Final source inventory/asset hashes must be read
  from the fresh handoff manifest and external coordinator receipt, avoiding
  self-referential hashes in the source being hashed. CI for uncommitted 9a
  has NOT run. Existing Phase 8 recipes/cache manifests are unchanged; no
  unnecessary Rust/oracle rebuild.

## Remaining gates

Native macOS ARM64 execution is **PENDING** user validation, after independent
artifact/code review; crossbuild/ad-hoc signature is not execution proof. The
binary-only checklist isolates absolute private HOME because macOS ignores
XDG_CONFIG_HOME, covering hash/version/relocation/config precedence/503/lock/
SIGINT/SIGTERM/restart. Optional attended real login belongs to the separate
production handoff; never capture device codes/tokens/account IDs/provider bodies.
Production login/live lifecycle and live eligible catalog remain **NOT PASSED**,
retained-probe generation 3 is never imported, and public release remains blocked.
Phase 9b resource/memory/load/leak/profiling work is **NOT IMPLEMENTED** here.
