# Native HTTP cancellation harness diagnosis (2026-10-16)

Scope: `TestNativeCancellationChainReleasesAdmission` and its new local test
helper only. No production/runtime/auth/toolchain changes, staging, commit,
push, live-provider request, or credential action. Starting HEAD:
`14648ff9ab610dc4a5bae915f5b2a2e469ba2e18`. All commands used Go 1.27.2, the
exact working-tree Makefile/go.mod pin.

## Finding and confidence

The original one-second assertion started at fake upstream generation, not at
an observed blocked downstream write. A client receive buffer of 1024 bytes
does not constrain the server's default TCP send queue. Each successful SSE
frame installs a fresh 30 ms write deadline. Thus upstream parsing/serialization
and filling the send queue are not bounded by that deadline.

This unsound harness assumption is demonstrated, not merely inferred:

- Instrumentation-only original harness, `-race`, nonreading subtest, count 20:
  all 120 cycles passed. Every cycle needed 40 text-frame attempts and accepted
  approximately 2.56 MB of TCP writes before the first partial write timeout.
  Generation-wait-to-timeout ranged from 0.399 to 0.489 seconds on this run.
  Log: `/tmp/native-cancel-original-trace-race20.log`.
- Controlled diagnostic: pace the otherwise same fake upstream by 20 ms/frame.
  Six cycles passed, taking 5.27 seconds overall. Log:
  `/tmp/native-cancel-paced-original-failure.log` (despite its exploratory name,
  this run **passed**).
- Change that controlled pace to 40 ms/frame, one repetition: the original
  assertion failed at 1.002811 seconds while all preceding downstream writes
  still succeeded, including the one at 0.991295 seconds. There was **no TCP
  timeout before the assertion**. Client cleanup then produced a broken pipe;
  upstream request context cancellation followed at 1.025788 seconds. Log:
  `/tmp/native-cancel-paced40-original-failure.log`.

These traces prove that the old test can report failed cancellation without
having exercised downstream backpressure at all. They do **not** reconstruct
the exact scheduling/interleaving in `/tmp/phase7-go127-race20-failed.log`, which
contained no timing trace. The harness assumption is a strong explanation of
that historical failure, not proof of its individual cause. No production
cancellation/body-close defect was demonstrated; this does not prove that every
possible runtime interleaving is correct.

Original test saved at `/tmp/native-cancel-original.go`. Temporary diagnostic
pacing and original high-volume tracing are not in the final test.

## Final harness

- Both local accepted TCP sockets set/check a small send buffer before net/http
  uses them. Downstream receive buffer remains 1024 bytes. The native client's
  injected local transport explicitly bounds its receive buffer to 64 KiB.
  Kernel minimum/rounding is allowed; assertions rely on observed writes, not an
  assumption that the effective socket buffer is exactly 1024 bytes.
- Per-request channels prevent cross-cycle notification confusion. An upstream
  gate holds text until the client has consumed response headers and promises
  no more reads, excluding text read-ahead as an accidental drain.
- The fake source is finite: at most four 64 KiB deltas in nonreading mode, one
  small delta in disconnect mode. It retains independent two-second watchdogs.
- The observer delegates all real writes, flushes, and deadline operations.
  Nonreading mode must witness a real partial TCP write with a timeout, a
  deadline still in the future at write entry, and expiry at/after that
  deadline. Its deadline must be no further than the unchanged runtime's 30 ms
  per-frame bound. The timeout must arrive within one second of the first TCP
  payload write. Missing backpressure is now an explicit test failure.
- Native request-context cancellation is observed independently of upstream
  writer/handler return. The cancellation and both handler exits share one
  unchanged one-second promptness budget from disconnect or actual TCP timeout.
  The downstream handler-exit witness follows admission-slot release.
- Both modes still run six cancellation cycles (more than admission capacity),
  require exactly one native generation per cycle, then require a healthy
  seventh request with exactly one `done`. No retry, continuation, or tool
  generation was introduced; no original failure mode was removed.

During helper development, an observer-only false negative was caught in
`/tmp/native-cancel-bounded-fast.log`: stopping an AfterFunc on handler exit can
suppress its callback after `Context.Done` becomes observable but before the
callback starts. The final observer uses a `sync.Once` report and checks the
actual request context on handler exit before stopping the callback. This was
an introduced test-observer issue, not evidence of a production defect.

## Validation and preserved failures

- PASS focused Go 1.27.2 race count 50, no retries:
  `go test -race ./internal/httpserver -run '^TestNativeCancellationChainReleasesAdmission$' -count=50 -timeout=100s -v`
  (external watchdog 120 seconds).
  `/tmp/native-cancel-bounded-race50.log`: 600 cycles, including **300 actual
  partial TCP timeouts** and 300 disconnects; all 600 native cancellation
  witnesses observed. Timed-out TCP writes blocked 24.055–29.990 ms; observed
  timeout-to-upstream-context-cancel latency was 0.142–0.737 ms on this host.
- PASS `make check build` (external watchdog 300 seconds): unit/race suites,
  vet, Staticcheck, nilness, govulncheck (no vulnerabilities), all four original
  fuzz smoke targets, and build. Other unchanged package results can be cached.
  `/tmp/native-cancel-go127-check-build.log`.
- Additional whole-package race count 20 initially FAILED at the command's
  130-second Go test watchdog (external watchdog 150 seconds), not a reported
  cancellation assertion: `/tmp/native-cancel-original-package-stress20.log`.
  Stack trace showed `TestForcedShutdownCancelsActiveRequest` one second into
  its intentional five-second drain. That existing test alone costs 100 seconds
  across 20 iterations; the command budget was insufficient for the whole suite.
- After inspecting that timeout and the existing drain test, one explicit
  completion run used a suite-sized 200-second Go test watchdog (external 220),
  with **no test assertion timeout changes**:
  `go test -race ./internal/credentials ./internal/application ./internal/catalog ./internal/httpserver ./internal/provider/codex ./cmd/atuin-ai-server -count=20 -timeout=200s -json`.
  PASS all six packages. HTTP server completed in 134.7 seconds; all 20 native
  cancellation tests passed. JSON reports exactly 100 seconds in forced drain
  and 19.02 seconds in body-limit tests, explaining the earlier command timeout.
  `/tmp/native-cancel-package-stress20-completion.jsonl`.
- PASS `git diff --check`; exact-toolchain gofmt reports no changes needed for
  either test file.

Scope verification at the diagnostic worker's handoff, before final delivery
restaging: HEAD and the Git index were unchanged (index SHA-256
`6f9df90dd21732925a7679c16f80edb07ee50a1950e19bf0093635418ec13574`).
Binary staged diffs and all pre-existing tracked unstaged diffs outside the
native cancellation test compare byte-for-byte with the starting snapshots.
Only the two test paths and this new note are delivery changes from this task.

The prior original failure and controlled failing trace remain preserved; they
are not relabeled green. No shared phase-progress/status document was edited.
Subsequent focused independent review CLEARED this test-only harness. FINAL
independent verification PASSed exact Go 1.27.2 `make check` (all tools and four
original 5s fuzz smoke targets), `make build`, diff check and all six runtime/CLI
race packages x20. The compiler/devtool focused review and OAuth/store/manager
security re-reviews are also cleared. These are code-checkpoint outcomes only,
not fresh production login/catalog/lifecycle evidence; those remain NOT PASSED
absent a separately recorded user report. Retained isolated-probe generation 3
is separate. Commit/CI identity belongs in the delivery report or later ledger.
