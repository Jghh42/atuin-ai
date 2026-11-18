# Phase 8 CI receipts / focused test-harness fixes

## Final corrected exact-HEAD delivery — BOTH GREEN

Coordinator verified reviewed fixture correction at
`a50af9a16ca08e9898077ad52a0b2551d5fa0a56` on clean main:

- [Pinned compatibility 38999649689](https://github.com/Jghh42/atuin-ai/actions/runs/38999649689): **SUCCESS**, including 20 actual CLI / 14 original oracle,
  source-prefix Bubblewrap/AppArmor setup and real kernel namespace probe,
  owned Docker/profile cleanup and validation aggregate.
- [Go checks 38999649761](https://github.com/Jghh42/atuin-ai/actions/runs/38999649761): **SUCCESS**, make-check/build.

This supersedes the historical pending/RED snapshots below, not their failure
evidence. Phase 9a was explicitly authorized after this exact-HEAD checkpoint.
No live production/catalog, native macOS or resource/release pass follows.

## Observed exact-HEAD attempt — FAILED, never rerun

Commit `6d023fef9779b13a8eb8cd745efab1a92995480b`
(`Validate pinned Atuin CLI and reference compatibility`):

- [Go checks 38988909350](https://github.com/Jghh42/atuin-ai/actions/runs/38988909350): **SUCCESS**.
- [Pinned compatibility 38988909352](https://github.com/Jghh42/atuin-ai/actions/runs/38988909352),
  [job 117266651520](https://github.com/Jghh42/atuin-ai/actions/runs/38988909352/job/117266651520): **FAILURE**.
- Compatibility `go-check`: SUCCESS. Hosted checksum-pinned source-prefix
  Bubblewrap, exact-path AppArmor load, **real namespace probe**
  (`PINNED_NAMESPACE_OK`), mandatory regressions, fresh Rust/Atuin build,
  actual **20/20 CLI cases**, and fresh original reference build: SUCCESS.
- Differential failed BEFORE its 14 subcases, at the `docker port` assertion:
  `exit status 1; Error response from daemon: page not found`.
- Independent lease cleanup and workflow's scoped Docker cleanup: SUCCESS.
  Exact job AppArmor profile removal: SUCCESS. Provenance upload: SUCCESS.
- `Phase8 validation aggregate`: correctly FAILURE, not a Go-only green pass.
  No workflow rerun, isolation relaxation or developer-host policy change.

Full job log retained at `/tmp/phase8-ci-first-pinned.log`; job/step metadata at
`/tmp/phase8-ci-first-run.json`. Those are local diagnostics, not fixtures.
Prior local/independent corrected-path 20 CLI / 14 oracle and repeated race
passes remain valid observations; they did not establish cold-runner CI success.
The hosted prefix/kernel path is now actually demonstrated by the successful
steps, NOT merely inferred from review or mocked profile tests. Overall Phase 8
CI and Phase 9 authorization remain BLOCKED until corrected exact-HEAD CI passes.

## Diagnosis: confirmed mechanism, bounded CI attribution

`docker()` used `CombinedOutput()` as a machine-readable return value. Docker
success stderr can contain cold-image pull progress or host warnings. `lease.create`
forwarded that combined text as a network/container ID without validation. The
oracle HTTP helper likewise decoded combined stdout/stderr as JSON.

A real, controlled local cold-image experiment (NOT a CI rerun) established:

- Docker client/server **29.7.2**, API **1.55**; isolated HOME, explicit local
  Unix socket. Version receipt `/tmp/phase8-docker-diagnosis/version.json`.
- Official `hello-world` amd64 manifest
  `sha256:d1a8d0a4eeb63aff09f5f34d4d80505e0ba81905f36158cc3970d8e07179e59e`
  was absent locally. An owned, labeled `--network=none` detached container was
  created only after registering the independent cleanup lease.
- Run succeeded with a valid 64-hex ID on stdout and **450 bytes of cold-image
  pull progress on stderr**. No image/cache pruning was used to force absence.
- `docker port` with the clean ID succeeded, empty stdout (no publication).
  The same ID contaminated with that success stderr produced EXACTLY
  `Error response from daemon: page not found`, exit 1. Scoped cleanup exited 0.
- Separate stream and port results are retained under
  `/tmp/phase8-docker-diagnosis/`; no private captures or binaries are committed.

Thus the merged-stream defect and exact error mechanism are confirmed, and are
strongly consistent with a cold CI sidecar image pull. The first job did NOT log
its successful create stderr, returned IDs, or actual Docker/API versions. Its
precise contaminated ID / warning cannot be reconstructed; do NOT claim an
observed swap warning, exact CI API mismatch, or unsupported empty-port behavior
from that generic 404. New diagnostics record successful stderr, validated IDs
and actual client/server/API versions so a later failure is attributable.

## Focused patch / invariant

- Capture stdout and stderr separately for Docker commands and helper envelopes.
  Only stdout is parsed; success stderr is retained in test diagnostics. Failures
  still fail with their exit error and BOTH streams, without filtering.
- Validate ONLY returned creation IDs as complete lowercase 64-hex identifiers
  before passing them to any later command. A malformed return leaves the
  uncertain-create lease pending, preserving bounded scoped late cleanup.
- Deterministic fake-Docker regressions emit warnings before/after success stdout,
  test IDs and HTTP JSON envelopes, reject malformed/multiple IDs and envelopes,
  preserve generic output and both failure streams/exit code.
- The original `docker port` no-publication assertion is UNCHANGED: any nonempty
  stdout fails, and command errors remain fatal. Clean-ID behavior was actually
  verified; no evidence justified replacing the check with a weaker fallback.
  Internal bridge, no host ports, labels/precreation cleanup and failure
  propagation remain intact. No runtime policy, source recipe, upstream
  application source, real credentials, live provider or Phase 9 work changed.

## Focused writer validation — PASS, review pending

- New deterministic warning/ID/envelope/failure regressions PASS:
  `/tmp/phase8-ci-docker-regressions1.log`.
- Current fixed-oracle manifest, actual **14/14 oracle cases PASS**:
  `/tmp/phase8-ci-docker-oracle1.log` ends `DOCKER_FIX_ORACLE14_EXIT_0`.
- Actual oracle + unfiltered integration **race x2 PASS** (28 oracle case runs):
  `/tmp/phase8-ci-docker-race2.log`.
- Full `make check`, `make build`, `git diff --check` PASS:
  `/tmp/phase8-ci-docker-validation1.log` ends
  `DOCKER_FIX_CHECK_BUILD_DIFF_EXIT_0`. No assertion/budget was relaxed.
- Build recipes, pins, source inputs and compiled Atuin are unchanged; no heavy
  Rust rebuild or new CLI run is needed for this differential-only helper fix.
  The first hosted actual 20-client pass and prior repeated local/independent
  client passes remain honestly separate from corrected differential tests.

The Docker helper patch subsequently CLEARED independent review and verification
and was delivered as `57bec1b12d8be78a9618bef55f251745602f874e`. Its CI results follow;
the original failure and limited warning attribution above are preserved.

## Second delivery — compatibility GREEN, Go checks RED

Exact HEAD `57bec1b12d8be78a9618bef55f251745602f874e`:

- [Pinned compatibility 38997902798](https://github.com/Jghh42/atuin-ai/actions/runs/38997902798):
  **SUCCESS**, including compatibility Go checks, checksum/source-prefix
  Bubblewrap + exact AppArmor setup + real namespace probe, fresh builds,
  **20 actual CLI cases / 14 original oracle cases**, scoped container/network
  and profile cleanup, and **Phase8 validation aggregate SUCCESS**.
- [Go checks 38997902805](https://github.com/Jghh42/atuin-ai/actions/runs/38997902805):
  **FAILURE**, in the offline Python watchdog fixture during `go test -race`.
  The earlier non-race Go test pass and compatibility pass do not override it.
- Overall checkpoint is **RED**, not inconclusive or ready for Phase 9. Neither
  run was blindly rerun. No live login or developer-host policy change is needed.

First failure retained at `/tmp/phase8-watchdog-ci-failed.log`, metadata at
`/tmp/phase8-watchdog-ci-run.json`; compatibility metadata at
`/tmp/phase8-watchdog-compatibility-success.json`. Assertion: final fake state
`network:true`, expected `false`, after the orphan watchdog exited. Python suite
reported **0.570s**, shorter than its configured **2s** pending-create drain.

### Confirmed fixture defect / attribution limits

The fake Docker and test parent concurrently read/rewrote a SINGLE JSON file
using `Path.write_text` (truncate then write). A fake listing can observe partial
JSON and fail; read-modify-write snapshots are also not independent resource
operations. The fixture checked only pidfd readability, NOT successful watchdog
exit, and discarded its stderr. Thus an early cleanup failure could look like a
completed watchdog, with the late network left behind and no useful diagnostic.

A controlled reproduction held a partially written JSON file while releasing an
in-flight fake network reader. The REAL unchanged watchdog exited **1 in 0.207s**
with `RuntimeError: networks listing failed`, reproducing the early exit / final
`network:true` shape. Script/log retained at
`/tmp/phase8-watchdog-controlled-repro.py` and
`/tmp/phase8-watchdog-controlled-repro.log`. This confirms the fixture race and
missing outcome check. CI's discarded stderr/exact scheduling cannot be
reconstructed; no unobserved JSON exception is retroactively asserted as fact,
and no production-cleanup failure is inferred from the synthetic assertion.

### Focused fixture-only correction

- Replace shared JSON snapshots with separate atomic existence markers, like
  independent daemon resources; creating one cannot overwrite another's state.
- The fake takes an EMPTY network snapshot and handshakes BEFORE returning it.
  Only then does the test create the late network and release the old empty
  response. A SUBSEQUENT cleanup pass is therefore required deterministically;
  no polling-container-deletion scheduling guess or arbitrary sleep is used.
- An independent observer reaps the REAL watchdog and records its numeric OS
  exit status and stderr. Both must indicate success, in addition to exact
  resource deletion / unrelated-resource preservation assertions.
- Cover completed `begin/end` leases as well as a killed owner with pending
  `begin`. A counterfactual scratch-only single-cleanup mutation MUST fail the
  specific late-network assertion, proving the witness cannot pass vacuously.
- The real watchdog, Go lease/precreation registration, Docker constraints and
  all timeouts are UNCHANGED. No runtime policy/source recipe/client behavior,
  provider credentials, global prune or unrelated-process cleanup changed.

### Fixture correction validation — PASS, review pending

- Focused pending/completed owner-death cases and single-cleanup negative
  mutation PASS: `/tmp/phase8-watchdog-fixed-focus2.log`.
- **20 suites, 4 workers, 60 scenarios** PASS (40 positive killed-parent cases
  plus 20 required mutation rejections), no timing retries:
  `/tmp/phase8-watchdog-parallel-stress2.log`.
- Unfiltered integration **race x10 PASS**, 28.524s:
  `/tmp/phase8-watchdog-frozen-integration-race10.log`.
- Full `make check`, build and diff checks PASS:
  `/tmp/phase8-watchdog-frozen-validation.log` ends
  `WATCHDOG_FROZEN_CHECK_BUILD_DIFF_EXIT_0`.
- Actual current-manifest original oracle **14/14 PASS**, with scoped cleanup:
  `/tmp/phase8-watchdog-oracle1.log` ends `WATCHDOG_FIX_ORACLE14_EXIT_0`.
- Real watchdog/lease code, CLI harness, source recipes, runtime and upstream
  sources remain unchanged. No heavy Rust rebuild or new CLI run was required;
  the actual hosted 20/14 compatibility success remains separate evidence.

Fixture patch awaits narrow independent review/verification BEFORE commit/push.
All required corrected exact-HEAD workflows must pass before Phase 9a packaging.
