# Phase 6 checkpoint — one-generation turn coordinator and SSE writer

2026-10-16. Implementation complete behind injected runtime dependencies;
**independently reviewed checkpoint, cleared for authorized direct-main delivery**.
Final review cleared the runtime and all three focused test-strengthening follow-ups.
Started from clean main
`8aa312ede2fcc3078683620c7efaade9fb00ec7e`; exact-HEAD Go checks run
`38956867298` was green per coordinator handoff. That is previous-SHA evidence,
not new Phase 6 CI. No live calls, new credentials, or Phase 7 lifecycle work.

## Seams and lifecycle policy

- `httpserver.NewWithRuntime` injects `credentials.Source`, `CatalogSource`,
  `provider.Provider`, and narrow preflight/write timeouts. Existing `New`
  remains standalone-safe 503, with no automatic isolated proof-store import.
  Catalog-only injection now returns safe 503 rather than the obsolete 501.
- Admission/body bounds are unchanged. Before headers: check atomic credential
  availability, read exactly one catalog snapshot, decode bounded input against
  its alias/default/provider IDs, and validate/generate the session UUID. The
  credential/catalog preflight is eight seconds by default; sources must honor
  cancellation. No downstream SSE/provider Stream on failed preflight/decode.
- `turn.Run` owns lifecycle and the sole terminal attempt via injected EventSink.
  It emits processing, thinking, text deltas; calls Provider.Stream exactly once
  (zero if initial sink fails). No tools execute, retry or automatic continuation.
  Generation has a separate 300-second bound so a provider timeout can still
  produce the generic error on a writable, connected downstream.
- The callback buffers/copies at most one bounded call batch, even if a buggy
  provider emits it then fails. Only after Stream success does ValidateBatch run
  capability/registry/schema/mixed-terminal/danger policy. Historical call-ID
  collisions are also rejected before any calls escape; original batch order is
  preserved. Duplicate batches, unknown events, invalid text and text after a
  batch fail closed. Serialized callbacks become closed at return; retained late
  callbacks return error without touching downstream or reopening a terminal.
- Text-only: done. Ordinary calls: tool_call+, waiting_for_tools, done. One
  suggestion: tool_call, done, no wait. Error: exactly one fixed generic
  `upstream_error` / `LLM request failed, please retry`, never done afterward.
  No raw provider/source/write error or client content is logged.
- Sink errors are sticky: immediately cancel upstream, no terminal write retry.
  Disconnected/canceled requests do not attempt further downstream writes.
  Stream owns native upstream body closure; disconnect and slow TCP tests prove
  cancellation. Injected providers/sources must honor context and return from
  callback errors; Go cannot preempt malicious non-cooperating implementations.
- The handler owns HTTP framing/writes via its sink. Each frame is named SSE,
  single-line json.Marshal data (including escaped newlines), blank delimiter,
  flushed individually; no id/retry. Required headers and consistent session
  header/done. ResponseController bounds each write AND flush to five seconds,
  independently of 300-second generation; unsupported deadline controllers fail
  closed. WriteTimeout supports short deterministic slow-client tests.

## Usage compatibility policy

Done contains every documented usage key. Individual Known flags preserve exact
input/output/total/cache-read values, including explicit zero. Missing counts
are JSON null, not invented zero; cache creation is always null because native
Codex Usage does not supply it. Native parser derives total only with known
input/output and validates consistency/overflow. Input includes cached reads;
they are not subtracted. All four cost fields are null. No hosted credits or
subscription price estimates. This intentionally distinguishes missing counts
from reference-server zero-fill and interoperates with the pinned CLI (which
ignores usage).

## Tests / validation

Original Apache-2.0 synthetic fixtures/goldens; no copied code or new NOTICE
material. Provenance is in `testdata/responses/README.md`.

- Runner sequence goldens: text, parallel ordered calls, terminal suggestion,
  dangerous-command escalation, provider callback-before-error, mixed/unknown/
  disabled/schema-invalid/duplicate/historical-collision batches, duplicate/
  late events after error, success and sink failure; sticky sink errors through
  final done and generic terminal-error writes (no consequent writes/retries);
  generation timeout (even a buggy provider
  returning success after cancellation), and usage known-zero/null accounting.
- Real TCP exact complete event-frame/header/body golden; UUID header/done
  consistency; flushed text visible while provider remains blocked; generic
  terminal failure with withheld calls; write and flush failure injection.
- Fake native Responses → Atuin: text, suggestion, each of seven local tools,
  parallel calls, full-history continuation without previous_response_id,
  upstream failure, schema/mixed/hosted rejection; exactly one upstream POST.
  Tool frames are decoded and compared for exact original call_id (not item ID),
  name, ordered raw arguments, large-integer and decimal-number lexemes.
  Continuation compares the full ordered history after the distinct TurnContext,
  including original parallel calls, reversed result order, failed-result prefix,
  exact argument lexemes, text blocks and original call/result correlation.
- Prestream malformed request/auth/catalog/credential-timeout failures never
  generate; existing body/admission saturation and loopback tests still pass.
- Real TCP disconnect promptly cancels provider. Nonreading TCP client with
  30ms write deadline cannot retain generation; no retries after write failure.
  A coupled native Codex → HTTP SSE test now covers both TCP legs for downstream
  disconnect and nonreading-client timeout: six cancellation cycles each must
  promptly close the actual fake-upstream request, make one POST per cycle, and
  permit a seventh successful generation, demonstrating admission-slot release.
- PASS `make check`: unit/race, vet, pinned staticcheck/govulncheck (no known
  vulnerabilities), four bounded fuzz smoke targets. PASS `make build`,
  `git diff --check`, targeted new coordinator/HTTP race stress x20 and native
  provider race stress x20. Focused test strengthening reran full check/build,
  diff check and coordinator/native-chain HTTP race stress x20; no runtime code
  changed. Local logs: `/tmp/phase6-{check,build,race}.log` and
  `/tmp/phase6-provider-race.log`; focused follow-up logs are
  `/tmp/phase6-hardening-{check,build,race}.log`.
  No exact-SHA Phase 6 CI exists before delivery.

## Retained blockers / later phases

Live eligible catalog **NOT PASSED** (discovery omitted replay-proven model;
subsequent one-candidate proof failed). No eligibility expansion or new live
calls. Phase 7 must implement production isolated login/store/refresh/logout,
account/reauth cache invalidation and no-config login-to-serve wiring. This is
not release ready. The initial Phase 5 decoder fuzz context-deadline failure
remains nonreproduced, cause **unknown**; unchanged fuzzing passed here, not
suppressed or diagnosed as infrastructure. Full context remains in the progress
ledger. Phase 8 readiness/budget stays recorded there: Rust/Cargo/CLI absent,
pinned source `/tmp/atuin-src` at `4ca8b77c8a526b8c294eb864a2b74c95c685c983`,
Rust 1.98.0 requirement/default AI features, Docker accessible, approximately
12 GiB repository disk and separate 6.4 GiB `/tmp` tmpfs constraints.
