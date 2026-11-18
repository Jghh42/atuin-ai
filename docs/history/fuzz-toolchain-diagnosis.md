# Fuzz budget-boundary diagnosis and exact toolchain pin

2026-10-16. Test/build toolchain follow-up only; no production authentication
logic, model eligibility, live gate, fuzz target, seed, assertion or error-status
handling is changed. Focused tooling review CLEARED the exact compiler/devtool
pins, CI selection and nilness coverage substitution. The separately reviewed
native-cancellation harness follow-up and final independent verification are
recorded below; this code checkpoint is cleared for authorized delivery.

## Preserved failures and confidence

The historical Phase 5 run and fresh Phase 7 delivery preflight both printed only
`context deadline exceeded` at the five-second FuzzDecode budget, without a
panic/assertion, saved failing input or stack. Original logs remain at
`/tmp/phase5-lifecycle-check.log` and `/tmp/phase7-delivery-check.log`. The latter
failed at 5.09s after 281348 executions; no unchanged rerun was used to clear it.

Read-only diagnosis established a concrete vulnerable branch in the Go 1.26.9
fuzz coordinator, matching [golang/go#75804](https://github.com/golang/go/issues/75804)
and upstream fix
[`5a957dc766aed011cdfe7c953987a2ff86c0829c`](https://go.googlesource.com/go/+/5a957dc766aed011cdfe7c953987a2ff86c0829c%5E%21/):

- The coordinator stops with the parent budget context's error, but the old guard
  compared only the child fuzz context's error. The parent's Done channel can
  close before cancellation reaches its child, so normal budget expiry can be
  retained as a failing DeadlineExceeded result.
- A finite native-context model using the unmodified old runtime reproduced
  **11 retained deadline errors in 20000 trials**. Eighteen separate bounded
  no-op/real-decoder fuzz experiments passed; those passes do not negate the
  vulnerable branch or diagnose an individual old failure.
- The original failed invocations were not instrumented. Their exact historical
  scheduling is still **not retrospectively proven**; no assertion that all
  possible decoder problems are ruled out is made. This source/model evidence,
  rather than earlier passing reruns, justifies fixing the known toolchain race.

## Official release verified, not guessed

On 2026-10-16, official [download metadata](https://go.dev/dl/?mode=json) listed
**go1.27.2** as the current stable Go 1.27.x patch. The official
[release history](https://go.dev/doc/devel/release#go1.27.2) dates it 2026-10-08
and describes security fixes in the go command, crypto/tls, html/template,
net/http, net/textproto and os. We did not select unpatched 1.27.0 or invent a
newer patch release.

The standard Go toolchain auto-download produced
`go version go1.27.2 linux/amd64`. Its actual unmodified
`$GOROOT/src/internal/fuzz/fuzz.go:129–137` contains the parent-AND-child error
guard and explanatory comment referencing issue 75804. The source file SHA-256
is `db5a9c6f3b26220eb9fb1c144caf012a555c6adef785cb9d185426e508d9d2aa`.
No installed source was patched. The official metadata's Linux amd64 archive
SHA-256 is `ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5`;
this is an archive checksum, not a claim that the auto-downloaded module ZIP or
individual executable has that same checksum.

## Repository disposition

- `go.mod` declares minimum **1.27.2** and is the single exact pin source.
- Make exports that exact `GOTOOLCHAIN` and checks GOVERSION for build/check/test
  targets. Normal auto-download is allowed, not silent fallback or a global
  environment/toolchain modification. `make fmt` uses the selected Go formatter.
- Both workflows select the exact version from go.mod with check-latest disabled,
  verify the actually installed local GOVERSION and pin later steps through
  GITHUB_ENV. No SDK/runtime dependency version is upgraded.
- Smoke campaigns remain **5s**, nightly campaigns **2m**, parallelism **2**.
  The nightly ten-minute watchdog, target matrix, minimization, seeds, assertions,
  crash artifacts and fail-on-error behavior remain unchanged. No retries,
  blanket deadline filtering, count substitution or duration inflation is added.
- Historical proof/checkpoint Go 1.26.9 environment records remain historical
  evidence, not silently rewritten to the new compiler.

## Development-tool compatibility follow-up

The FIRST new-compiler full check passed unit/race/vet, then Staticcheck v0.7.0
failed on export data version 5 (maximum supported 2). This was not retried or
ignored: `/tmp/phase7-go127-check.log` preserves it. A separately authorized check
of the old govulncheck v1.1.4 then panicked in x/tools v0.29.0's SSA builder on
`*ast.KeyValueExpr`; `/tmp/phase7-go127-govuln-old.log` preserves that failure.

Maintained release/module/source evidence selected exact DEVELOPMENT-tool pins:

- Staticcheck **v0.8.2 / 2026.2.2**: [official release](https://github.com/dominikh/go-tools/releases/tag/2026.2.2)
  explicitly supports Go 1.27.2's updated export format; not a guessed tag.
- govulncheck **v1.9.0**: latest stable official module/tag metadata, with x/tools
  **v0.52.0**. Its tagged source defines export V5 and tests Go 1.27 promoted-field
  struct literals, addressing the compiler/SSA incompatibility rather than
  hiding a vulnerability finding.
- Staticcheck 2026.2 retired SA5011 upstream. To avoid silently losing nil-pointer
  analysis, the lint target ALSO runs the maintained x/tools **v0.52.0** nilness
  command, the upstream-recommended similar alternative. Exact diagnostic parity
  is not asserted. [Module checksums and source references](../provenance/dependencies.md).

No application/runtime SDK module or go.sum dependency is upgraded. No `@latest`
floating pin, check filter, skip, old-compiler fallback or retry is introduced.
Focused review explicitly cleared this nilness coverage substitution.

## Fixed-toolchain verification and disposition

All commands below used Go **1.27.2**. Tool versions printed Staticcheck
`2026.2.2 (0.8.2)` and `govulncheck@v1.9.0`, also built with Go 1.27.2.

- Fresh full `make check` and `make build`: **PASS**, including Staticcheck,
  explicit nilness, govulncheck (no vulnerabilities) and ALL FOUR original 5s
  fuzz targets. Log: `/tmp/phase7-go127-devtools-check.log`.
- Five predeclared independent FuzzDecode campaigns, each `-fuzztime=5s` and
  `-parallel=2`: **5/5 PASS**, 968008 total executions. Logs:
  `/tmp/phase7-go127-fixed-fuzz/run-1.log` through `run-5.log`. The set stopped
  on any failure by construction; none failed and none was retried. These
  repetitions do not retrospectively instrument the old scheduling.
- Race x20: credentials, application, catalog, native provider and main CLI
  **PASS**. The same command additionally included httpserver, which **FAILED**
  `TestNativeCancellationChainReleasesAdmission/nonreading`, cycle 4:
  `downstream nonreading did not cancel native HTTP`. The entire race command
  therefore **FAILED**, not a full race pass. Preserved log:
  `/tmp/phase7-go127-race20-failed.log`. Delivery paused for separate investigation;
  no blind rerun, runtime fix, weakened timeout or compiler-regression claim was
  made. The subsequent test-harness disposition is below.
- Darwin arm64/amd64 cross-builds: **PASS**; execution is NOT tested.
  Linux binary build metadata confirms Go 1.27.2. No privileged owner-mutation
  test pass is claimed. Formatting and diff checks: **PASS**.

## Reviewed native-cancellation follow-up and final verification

The [native cancellation diagnosis](../provenance/native-cancellation-diagnosis.md)
proved the old harness's implicit backpressure assumption unsound: an explicit
40ms/frame controlled case hit its assertion while every TCP write still
succeeded. The exact historical uninstrumented interleaving remains unproven.
Only the two test/harness files changed, with bounded real sockets, header/read
handshakes and an actual partial-TCP-timeout witness. Runtime cancellation,
admission, deadline values and no-retry behavior were not changed.

Independent focused review CLEARED this harness. Worker race x50 covered 300
actual TCP timeouts plus 300 disconnects, followed by successful all-six-package
race x20 with an adequately sized command watchdog; preserved diagnostic timeout
and its measured disposition are in the note. No test assertion timeout was
inflated to hide the original failure.

FINAL independent verification then PASSed exact Go 1.27.2 `make check` (all
three analysis tools, vulnerability scan and all FOUR original 5s fuzz targets),
`make build`, diff check and credentials/application/catalog/httpserver/native-
provider/CLI race x20. The exact toolchain/source SHA was independently
reverified. Focused tooling AND harness reviews are CLEARED for delivery.

No runtime auth logic/SDK dependency changes, provider/credential/device actions,
production-live pass or Phase 8 execution occurred in these follow-ups. The old
failures remain preserved, not relabeled successes. Production live/catalog gates
remain NOT PASSED; final commit/CI identity is recorded by the delivery report,
not inferred from any earlier run or index snapshot.
