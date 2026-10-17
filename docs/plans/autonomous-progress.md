# Autonomous implementation progress

Updated 2026-10-17. User-authorized sequential work; checkpoints require review
and delivery before moving on. This file distinguishes code from live gates.

| Phase | Status / evidence |
| --- | --- |
| 0 | Historical isolated native login, `gpt-6.1-sol` streamed text/tool full-history replay and refresh/restart PASSED; production integration not implied. [Evidence](../proofs/codex-phase0.md). Independent SIWC alternative remains blocked. |
| 1–2 | Historical bounded shell and decoder completed; static catalog/8080 assumptions now superseded by Phase 3a, not retroactively claimed discovery behavior. [Phase 2](phase2-status.md). |
| 3 | Prompts/tools/safety implemented, no tool emission. [Checkpoint](phase3-status.md). |
| 3a | Independently reviewed implementation checkpoint, cleared for authorized direct-main delivery. Native HTTP discovery/required-field decoding demonstrated, but eligible live catalog **NOT PASSED**: returned catalog omits replay-proven `gpt-6.1-sol`. Injected tests and independent verification pass. Production credential wiring deferred to 7; full phase exit is not claimed. [Checkpoint](phase3a-status.md). |
| 4 | Independently reviewed native Responses request projection/HTTP opening checkpoint, cleared for authorized direct-main delivery. Deterministic injected tests; no parser/Stream implementation or live attempts. [Checkpoint](phase4-status.md). |
| 5 | Native bounded Responses SSE provider implemented behind injection; six initial findings/shared helper and lifecycle-output follow-up independently reviewed and cleared for authorized direct-main delivery. No live calls. [Checkpoint](phase5-status.md). |
| 6 | Implemented behind injectable credentials/catalog/provider; one-generation runner, atomic CLI gate, bounded SSE writes and real TCP tests. Independent final review cleared runtime and all three focused test-strengthening follow-ups; authorized direct-main delivery checkpoint. Standalone remains safe 503. [Checkpoint](phase6-status.md). |
| 7 | Production native OAuth/isolated descriptor-rooted store, durable pre-refresh intent, serialized rotation, explicit auth CLI and shared catalog/SSE composition implemented. Deterministic fake lifecycle tests; two initial reviews found no criticals but required concurrent-Close and Unicode-fold fixes plus formatter strengthening. Both independent fix re-reviews CLEARED with no new blockers; independent post-fix verification passed. Reviewed code checkpoint cleared for authorized direct-main delivery, not a live phase exit. Fresh production login/live lifecycle and live eligible catalog **NOT PASSED**. [Checkpoint](phase7-status.md). |
| 8 | Implemented source-pinned unmodified Atuin PTY and original reference-source differential suites, synthetic OAuth/discovery/Responses only. All seven local tools include real isolated daemon output retrieval; default offline deterministic corpus plus opt-in hard gates/CI. Local validation/provenance and preserved harness failures: [Phase 8](phase8-status.md). All final independent re-reviews CLEARED corrected confinement/build/buffering/cleanup, oracle projection/session/policy/text normalization and exact-path hosted-CI setup. Independent current-artifact 20 CLI/14 oracle plus CLI race x2 (40 cases), oracle race x2 (28), unfiltered integration race x2 and integration race x10 PASSED. Delivered `6d023fe`; first exact-HEAD Go checks passed and hosted namespace/build/20-CLI steps passed, but differential Docker machine-output handling failed and aggregate is RED. Focused stderr/stdout/ID regression fix awaits independent review and new exact-HEAD CI; [first CI receipt](phase8-ci-receipt.md). Historical unconfined passes are NOT isolation proof. Production live/catalog gates remain NOT PASSED. |
| 9 | Not implemented; packaging/resource and release gates remain. |

## Phase 7 delivered baseline / Phase 8 boundary

Phase 8 began from clean main `b990c404e4582246a8c2ff0a067b33f14b148ca0`.
Coordinator-verified exact-HEAD Go checks run `38974738689`: **success**.
This is prior-SHA evidence, not CI for Phase 8's delivery SHA. The user
explicitly authorized continued Phase 8 implementation, with deterministic fake
accounts/backends and no new live probes. No fresh production login outcome was
reported. Existing production lifecycle/catalog gates stay **NOT PASSED**;
retained probe generation 3 is separate and never imported. Phase 8 adds no
runtime configuration, source fork, credentials or Phase 9 packaging/resource
claim. Independent review precedes any commit/push.

## Phase 8 review follow-up — final reviews CLEARED; delivery CI separate

Actual CLI runs paused on the critical ancestor discovery/interpolation finding.
Corrected runs now require an fd-pinned Bubblewrap filesystem/PID boundary,
without mounted host ancestors/HOME, plus metadata/symlink rejection before each
child launch. Synthetic ancestor/project/skill and command-execution regressions
were added; no private ancestor contents inspected and no incident inferred.
Fresh source/toolchain/Cargo/target builds replace markers/overlays; both fake
backends reject semantic projection mutations, sessions validate before UUID
normalization, intentional policy payloads compare completely, and an independent
owned-label lease watchdog handles failed/late creates and killed parents.
Secret-free PR/push/manual CI now has a failure-propagating validation aggregate;
admin branch rules and actual CI execution remain external/unproven. CLI seams
subsequently CLEARED re-review. Oracle's remaining P2 text-metadata normalization
loss is fixed with direct and positive/policy mutation regressions. CI now builds
checksum-pinned Bubblewrap into a private prefix, requires a real namespace probe,
and implements exact-path hosted-job-only AppArmor userns permission plus always-
cleanup (final narrow review CLEARED; first delivery CI subsequently PASSED that
actual hosted-kernel setup/probe and all 20 CLI cases, while failing differential
setup and correctly keeping the aggregate red). Local
prefix source build/version/inventory succeeded but its namespace probe is
BLOCKED by developer AppArmor; no developer-host policy change forced a pass.
The reviewed system-bwrap path remains unchanged unless explicitly configured. No runtime
policy, real login, eligibility or Phase 9 changes. See [corrected evidence and
preserved failures](phase8-status.md). All independent reviews and final local
compatibility verification have now passed; normal main delivery is authorized.
First delivery CI exposed merged Docker stdout/stderr being used as IDs/JSON.
A controlled cold-image run reproduced the exact daemon 404 from a tainted ID;
first-job IDs/warnings were not logged, so precise CI attribution is bounded.
The focused fix preserves diagnostics/exit failures and validates create IDs;
new deterministic regressions and [receipt](phase8-ci-receipt.md) precede review.
Do not start Phase 9 until ALL corrected exact-HEAD push workflows, actual compatibility,
validation aggregate and scoped cleanup succeed. Existing branch protection
is external and was not changed. No real-provider/live eligibility claim follows.

## Fuzz toolchain and cancellation-harness follow-ups — CLEARED

Delivery preflight under Go 1.26.9 again failed the unchanged five-second
FuzzDecode command with a naked context deadline error; no commit/push or blind
rerun followed. The preserved new log is `/tmp/phase7-delivery-check.log`.
Read-only diagnosis identified the exact vulnerable coordinator guard and
[upstream issue/fix](fuzz-toolchain-diagnosis.md), with a native-context model
reproducing 11 leaked deadline errors in 20000 trials. Individual historical
scheduling was not captured and remains retrospectively unproven.

Official current download metadata and release history verified Go **1.27.2**
(released 2026-10-08, security fixes). Its normally downloaded, unmodified actual
source contains the parent-plus-child guard at internal/fuzz/fuzz.go:129–137.
The project minimum and exact Make/CI pin now use that release. All 5s smoke/2m
nightly budgets, assertions, seeds and failure behavior remain unchanged; no
count replacement, error filter, retry wrapper or SDK dependency upgrade.
Production auth logic is unchanged. Staticcheck v0.7.0 then failed on Go 1.27
export v5; the old govulncheck v1.1.4 also failed in its obsolete SSA builder.
Both first failures are preserved. Verified maintained DEVELOPMENT-only pins
are Staticcheck v0.8.2 and govulncheck v1.9.0; an explicit x/tools v0.52.0
nilness pass preserves nil-dereference analysis retired upstream from Staticcheck.
No app/runtime SDK dependency or go.sum is widened.

Fresh `make check`/build with Go 1.27.2 and compatible tools PASS, including all
four unchanged fuzz targets; five additional independent decoder 5s campaigns
PASS. Darwin cross-builds and credentials/application/catalog/native-provider/
CLI race x20 PASS. An additional httpserver race check FAILED native cancellation
for a nonreading downstream (cycle 4); that original combined command FAILED and
remains preserved. Subsequent [controlled TCP diagnosis](../provenance/native-cancellation-diagnosis.md)
proved the old harness could assert cancellation failure before any write timeout
had occurred; the exact historical interleaving remains unproven. A test-only
bounded socket/handshake/partial-timeout witness fix preserved cancellation,
admission, one-second event-to-cleanup assertions and no retries. Focused
independent review CLEARED it; worker race x50 exercised 300 timeouts and 300
disconnects, and all-six-package race x20 completed successfully.

Focused Go/toolchain/devtool review also CLEARED, including the explicit nilness
coverage substitution. FINAL independent verification PASSed exact Go 1.27.2
`make check`/build/diff and credentials/application/catalog/httpserver/native-
provider/CLI race x20. No functional auth/runtime code changed in these follow-ups.
The code checkpoint is cleared for authorized delivery; [sources and preserved
failures](fuzz-toolchain-diagnosis.md) remain distinct from final verification.
No fresh production live/catalog pass, root ownership-mutation pass, macOS
execution or Phase 8 execution is inferred.

## Phase 5 decoder-fuzz validation anomaly — historical trace unproven

The original record below preserves what was known at that checkpoint. The
later source/model diagnosis above supplies concrete evidence of a known Go
coordinator race, not retrospective instrumentation of this individual run.

The first post-lifecycle-fix `make check` stopped at the unchanged command:

```text
go test ./internal/atuin -run='^$' -fuzz=FuzzDecode -fuzztime=5s -parallel=2
fuzz: elapsed: 0s, gathering baseline coverage: 0/267 completed
fuzz: elapsed: 0s, gathering baseline coverage: 267/267 completed, now fuzzing with 2 workers
fuzz: elapsed: 3s, execs: 166199 (55386/sec), new interesting: 1 (total: 268)
fuzz: elapsed: 5s, execs: 287396 (57687/sec), new interesting: 3 (total: 270)
--- FAIL: FuzzDecode (5.11s)
    context deadline exceeded
FAIL
exit status 1
FAIL	github.com/Jghh42/atuin-ai/internal/atuin	5.113s
make: *** [Makefile:22: fuzz] Error 1
```

Original local output is retained at `/tmp/phase5-lifecycle-check.log`. It has
no stack trace, assertion message, saved-failing-input path, or reproducer.
Read-only checks found no repository `testdata/fuzz` artifacts. The Go fuzz
cache for `internal/atuin/FuzzDecode` exists, but coverage-cache files are not
identified as failing inputs by this log. No saved failing corpus is known.

An unchanged full rerun (`/tmp/phase5-lifecycle-check-rerun.log`) completed
270/270 baseline entries, ran 285663 executions, and passed at 5.095s. The
subsequent build/diff check and provider race x20 passed too. This does **not**
prove an infrastructure/budget cause or rule out a decoder problem: exact cause
remains **unknown**. No deadline/timeout suppression or decoder changes were
made. Independent verification subsequently passed five isolated decoder fuzz
runs, full `make check`/build, focused race x10 and targeted lifecycle/regression
tests. The nonreproduced initial failure remains recorded with unknown cause;
these passes do not establish an infrastructure/budget diagnosis.

## Phase 7 independent-security-review follow-up

Both read-only initial reviews completed with no criticals and two must-fix
warnings: concurrent Close could return before shutdown/store release, and
ToLower-based duplicate detection missed Go Unicode-fold aliases. The sole
writer implemented a shared closing/completed barrier and close result, exact
SimpleFold-orbit duplicate keys, and fmt.Formatter redaction for unsupported
verbs without changing private-store JSON. Added finite persistence/store-close
handshake regressions, Unicode/root/nested/numeric cases and expanded fmt/slog
coverage. Details and post-fix checks: [Phase 7 status](phase7-status.md).

Original independent full check/race x20/Darwin cross-build results were green.
Post-fix writer `make check`/build/diff checks, Darwin cross-builds and all six
runtime/CLI race packages x20 now also PASS; targeted new regression race x20
passed. Logs are in Phase 7 status. Both independent security fix re-reviews
then CLEARED all fixes with no new blockers. Independent post-fix full
check/build/diff and credentials/application/catalog race x20 also PASS. No root
ownership-mutation test pass or macOS execution is claimed. This reviewed code
checkpoint is cleared for authorized direct-main delivery; production live
outcomes remain separate. The user already authorized own login, automatic
discovery and initial serve/models metadata checks: no redundant approval per
ordinary bounded catalog read. Blind retries, candidate inference, eligibility
expansion and Phase 8 remain outside the attended login handoff. Delivery is
separately authorized; its exact SHA/CI result belongs in the delivery report or
next ledger entry.

## Fresh retained-probe recheck — separate from production gates

The user returned and offered attended validation. While two read-only security
reviews and independent verification were still running, a separately authorized
bounded EXISTING-PROBE check passed: initially unexpired generation 2; completed
text, synthetic call and full-history replay; exactly one durable refresh to
generation 3; a NEW process passed post-refresh text/tool/replay/call correlation.
Exactly six inference generations total, no retries, no login/logout/discovery
and no repository/runtime changes from that probe check. This is coordinator-
supplied sanitized evidence, not a repeat run by the documentation writer.
[Evidence](../proofs/codex-retained-probe-2026-10-16.md).

It is NOT fresh production login, production refresh/store validation or an
eligible-catalog pass. Both production live lifecycle and live eligible catalog
remain **NOT PASSED**. No credentials may be imported from the retained probe.
[Attended production-login handoff](phase7-attended-login-handoff.md) is prepared
for the user's own terminal; security-fix re-review is now cleared. The user has
already authorized the ordinary bounded login/discovery/initial serve/models
workflow. Any later production outcomes are recorded separately. This PROBE
evidence alone does not authorize extra inference, eligibility expansion or
Phase 8 execution; code delivery is separately authorized.

## Phase 7 validation follow-up — stream test ownership assumption

One late Phase 7 race x20 sweep failed the pre-existing
`TestBlockedReadClosedOnTimeoutAndCancel` with `reader left blocked`; original
output `/tmp/phase7-final-race20-failed.log`. The original test lacked per-state
diagnostics, so its precise failing interleaving remains **unproven**. Its
sleep-based cancellation could occur before transport/body acquisition, yet the
assertion unconditionally required that unacquired body to be closed. A new
deterministic pre-open regression establishes that this assumption is unsound,
not that this was definitively the observed failure's cause. A diagnostic-only
x300 rerun passed (not a diagnosis). The test now handshakes on actual blocked
Read entry and injects cancellation/deadline causes with finite close/join guards;
real TCP inactivity/generation tests remain unchanged. No stream runtime change.
Focused race x50 passed. Final full `make check`/build/diff checks, Darwin
arm64/amd64 cross-builds and credential/application/catalog/httpserver/native
provider/CLI race x20 all PASS. macOS execution is not tested; owner-mismatch
mutation test is explicitly skipped on this non-root runner. No security-review,
production-live, commit/push or exact-HEAD CI pass is inferred.
See [Phase 7 final results](phase7-status.md).
This is separate from, and does not explain, the earlier FuzzDecode anomaly.

## Phase 3a delivery checkpoint

Delivered on main as `2431b10fe9fb8287de008f0ad0ceacca6f5f096b`.
Exact-HEAD GitHub Actions run `38950960197`: completed **success** (verified
read-only before Phase 4 implementation). Live eligible catalog remains
**NOT PASSED**; the green deterministic CI does not clear that gate.

## Phase 6 delivery checkpoint

Phase 7 started from clean main `14648ff9ab610dc4a5bae915f5b2a2e469ba2e18`.
Exact-HEAD Go checks run `38959509541`: **success** (coordinator-verified
handoff). This is previous-SHA evidence, not Phase 7 CI. Phase 7's reviewed code
checkpoint is now cleared for delivery; its exact SHA/CI result belongs in the
delivery report or a later ledger entry, not an own-hash assertion here.

## Phase 5 delivery checkpoint

Phase 6 started from clean main `8aa312ede2fcc3078683620c7efaade9fb00ec7e`.
Exact-HEAD Go checks run `38956867298`: **success** (coordinator-verified
handoff). This is previous-SHA evidence, not Phase 6 CI. Independent final
review cleared the Phase 6 runtime and all three focused follow-ups: coupled
native TCP cancellation/admission tests, runner late callbacks/final sink
failures, and exact tool/continuation payload assertions. Follow-ups changed no
runtime code. Independent verification passed full check/build/diff and runtime
race x20; focused follow-up full checks and race x20 also passed. This checkpoint
is cleared for authorized direct-main delivery; its exact SHA and CI result
belong in the delivery report. No new live calls were made.

## Phase 4 delivery checkpoint

Delivered on main as `2b446ac047d4a6acbb1f41b635dcc4625491d5ea`.
Exact-HEAD GitHub Actions run `38953006689`: completed **success** (verified
read-only before Phase 5 validation). The eligible-catalog gate remains
**NOT PASSED**. Phase 5 is independently reviewed and cleared for delivery;
its exact SHA and CI result belong in the delivery report.

## Phase 4 validation checkpoint

Request projection only, independently reviewed and cleared for authorized
direct-main delivery. Exact SHA and CI outcome belong in the delivery report.
No Phase 5 parser/state machine, Phase 6 integration, or Phase 7 store work.
`make check`, `make build`, and `git diff --check`: PASS. Full check includes
unit/race tests, vet, pinned staticcheck/govulncheck (no vulnerabilities), and
bounded existing fuzz smoke tests. Results/interfaces/next steps are recorded
in [Phase 4 status](phase4-status.md). Independent review found a dangling
terminal-suggestion replay and credential-lookup cancellation classification.
Both fixes cleared by independent final review: suggestions replay as labeled non-execution assistant
text with original raw arguments, never synthetic outputs; context errors are
preserved immediately after lookup. Added before/after-turn and blocked-source
regressions, and pinned/read Pi's `dist/api/transform-messages.js` dependency.
No new live attempts or eligibility changes; the catalog blocker is still
**NOT PASSED**. Post-fix `make check`, `make build`, `git diff --check`, and
`go test -race ./internal/provider/codex -count=20`: PASS. Independent validation
also passed full check/build and repeated race tests. No new review findings.

## Phase 3a validation checkpoint

- `make check`: PASS: `go test ./...`, `go test -race ./...`, `go vet ./...`,
  pinned staticcheck and govulncheck (no vulnerabilities), plus bounded decoder,
  batch-validator and safety fuzz smoke tests.
- `make build`: PASS.
- `git diff --check`: PASS.
- `go run ./cmd/codex-proof discover --approve-live`: exit 1, **NOT PASSED**
  eligible-catalog gate. Historical safe report: native HTTP 200; required
  field/type decoding succeeded (not full metadata validation); one model;
  integer priority; proven model absent. [Redacted evidence](../proofs/codex-discovery-phase3a.md).

Source pin: OpenAI Codex `96eb400d1231f0199e96f640c740096087cf3e9b`, hashes and
retained license/NOTICE in [provenance](../provenance/codex-discovery.md).
No public `/v1/models`, third-party credential reuse, device-code interaction,
raw logs, static fallback or unproven model enablement occurred. Existing
isolated access token was current; no refresh or new login was performed.

A coordinator-authorized follow-up ran `prove-discovered --approve-live` once
against one automatically selected actual picker candidate, using the same
isolated native credential. It failed before text proof completed (all proof
booleans false); no tool/continuation proof passed, no candidate ID was retained,
and no model was enabled. No retries or alternate candidates were attempted.
The precise initial failure classification was not retained and is not guessed.
The live gate remains **NOT PASSED**. [Bounded replay evidence](../proofs/codex-discovered-replay-followup.md).
Follow-up `make check`, `make build`, and `git diff --check`: PASS.

## Open blockers / decisions

1. Live eligible discovery needs upstream visibility of a replay-proven model
   or separately reviewed replay proof before any allowlist expansion. Do not
   silently treat discovery as tool/stateless compatibility proof.
2. No verified model release metadata/version convention exists in inspected
   source; numeric upstream priority fallback is intentional. ETag revalidation
   is not live verified or implemented.
3. Phase 7 now wires explicit fresh application login and native discovery/chat
   at port 17391 with no automatic proof-store import. Missing credentials or an
   eligible catalog still yields safe 503. Both independent security re-reviews
   cleared the code; fresh production login/live lifecycle and eligible discovery
   remain NOT PASSED at this checkpoint.
   This is not a release-ready or live-validated startup claim.
4. Independent final review cleared all five fixes and the bounded
   `prove-discovered` extension. Independent verification passed `make check`,
   `make build`, `git diff --check`, and `go test -race ./internal/catalog -count=20`.
   Exact commit identity and push/CI outcome belong in the delivery report, not
   inside the commit being identified. Under user authorization, Phase 4+ may
   continue with injected providers after the exact-SHA delivery checkpoint,
   despite the explicitly retained live blocker.

## Independent-review fixes (no further live probing)

- Hard refresh errors now invalidate completed stale-flight results as well as
  the stored snapshot. Controlled regression: stale completion → newer
  unauthorized/metadata/override failure → delayed old consumer rejection.
- Credential lookup success **and error** are checked under lock against the
  captured lifecycle revision. Regression: obsolete delayed error across
  completed reauthentication/new discovery cannot clear the new cache.
- Shared metadata validation is independent from replay eligibility; malformed
  diagnostic tests cover enum, ID, duplicate and string-bound failures. The old
  live `schema_validated:true` claim is explicitly narrowed to required-field
  decoding; full metadata validation is not retroactively claimed live-tested.
- Configuration reads one bounded 64 KiB byte snapshot, eliminating the double
  file-read race; replacement/size regressions pass.
- Counting/changing HTTP catalog tests observe one read per request and decoder
  resolution through a private observer, without public test APIs.
- Post-fix `make check`, `make build`, `git diff --check`: PASS. Live gate still
  **NOT PASSED**. No further live probes were performed.
- Independent final review cleared `internal/codexproof/discovered_replay.go`
  (`ProveDiscovered`, `proveDiscovered`, `selectReplayCandidate`), its tests and
  the `prove-discovered` branch of `cmd/codex-proof/main.go`.

## Historical pre-Phase 8 readiness (superseded)

This retained inventory described the Phase 7 handoff, not current availability.
Phase 8 subsequently provisioned isolated pinned artifacts and ran the actual
CLI/oracle suites; [current evidence](phase8-status.md) supersedes the initial
missing-toolchain/build observations below.

- Pinned Atuin source is available at `/tmp/atuin-src`, exact HEAD
  `4ca8b77c8a526b8c294eb864a2b74c95c685c983` (verified).
- `rustc`, Cargo and an Atuin executable are not installed/on PATH. Phase 8
  cannot yet claim real-CLI execution; toolchain/binary provisioning is needed.
- Docker daemon is accessible (server 29.7.2). This is test-oracle readiness,
  not a deployment decision or evidence that any reference image was built.
- `/usr/bin/script` provides a PTY; `expect` is absent. Choose/provision a
  bounded PTY harness rather than claiming existing TUI automation.
- Approximately 12 GiB is free on the repository filesystem. Rust builds,
  Docker layers/images and caches need explicit disk budgeting/cleanup; avoid
  unbounded concurrent provisioning or retaining multiple large build trees.
  `/tmp` is a separate approximately 6.4 GiB tmpfs; budget source/build staging
  accordingly. Pinned source requires Rust 1.98.0 and default features include
  AI. No Rust toolchain/image provisioning or Phase 8 implementation was done here.
