# Phase 8 first delivery CI / focused Docker harness fix

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

Patch/tests require independent review BEFORE any corrective commit/push.
Actual corrected CI has NOT run; this receipt is not a green checkpoint.
