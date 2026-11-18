# Phase 4 checkpoint — native Responses request projection

2026-10-16. Independently reviewed implementation checkpoint, cleared for
user-authorized direct-main delivery. Exact SHA and CI outcome belong in the
delivery report, not inside the commit being identified.
Only request projection and HTTP opening are implemented, **not** a complete
streaming provider, SSE parser, turn runner, or production OAuth lifecycle.

## Behavior / interfaces

- `internal/provider`: narrow `Request` (normalized Atuin conversation), `Event`
  (text delta or completed tool-call batch), `Usage`, and `Provider.Stream`
  contract for Phases 5–6. Stream success requires explicit completion; sink
  failure cancels; no automatic retries. There is no Codex Stream implementation
  yet and HTTP 200 is deliberately not reported as generation success.
- `codex.Project`: `prompt.Build` policy becomes instructions. A separate user
  message containing bounded/escaped `prompt.TurnContext` is **input index 0 on
  every request**, then every historical block in original order. User and
  assistant text use message shorthand, as demonstrated by the native proof.
- Ordinary assistant calls become `function_call` with original `call_id` and raw JSON
  argument string (including numeric lexemes); no output-item IDs are invented.
  Results become `function_call_output`. Failed results use the original,
  deterministic prefix `Tool execution failed: `, including empty failure text.
  Correlation uses outstanding IDs across the whole history, not adjacency;
  ordinary unresolved calls remain rejected. Terminal `suggest_command` history
  is the explicit exception: assistant **text**, labeled as a suggestion and
  not evidence of execution, followed by the original raw argument JSON. It
  never becomes a dangling function call or an invented tool output. Unlike
  Pi's `transform-messages.js`, no synthetic missing-result repair is performed.
  Full history is sent on every continuation, with
  no previous_response_id, prompt cache/session key, or Atuin session mapping.
- Codex fields mirror the pinned adapter's baseline: store false, stream true,
  encrypted reasoning include, low text verbosity, auto tool choice, parallel
  calls true; flat capability-gated function tools with **strict:null** (not
  public strict mode/schema rewriting or SIWC namespaces). No opaque state is
  fabricated; eligibility remains the catalog's responsibility.
- `codex.RequestClient.Open`: one native POST with atomic `Source.Current`
  token/account pair; Bearer, account, originator atuin-ai, application UA,
  responses experimental beta, JSON Content-Type, event-stream Accept. No
  redirects/retries, upstream body/error passthrough, probe-store imports, or
  tool execution. Fixed safe errors; context cancellation/deadline is checked
  immediately after credential lookup (even if lookup fails or returns a valid
  credential), before classifying errors or starting HTTP.
  HTTP generation is bounded to 300 seconds, including response consumption.
  Caller must close the body and keep context alive through parsing. Phase 5
  must add bounded SSE parsing and the 60-second inactivity policy.
- Decoder normalization remains the ingress boundary. Defense-in-depth bounds
  before rendering cover 4 MiB aggregate text/arguments/config, history/message
  counts, 256 KiB arguments, 32 calls per assistant message, and a 32 MiB maximum
  encoded request after escaping. Decoder retains token/JSON structure checks.

## Tests / provenance

`internal/provider/codex/request_test.go` contains original synthetic fixtures
and expected request shapes, not captured user/model/account data. Local HTTP
servers verify exact JSON and headers for text, suggestion, single/parallel
calls, failed/reversed outputs and interleaved multi-turn continuation. Tests
also cover every-request history/context, capability gating, escaped/truncated
context, default endpoint, rotating atomic credentials, request bounds, safe
status/transport/auth failures, redirect rejection/no retries, and cancellation
before headers, during body consumption, and while credential lookup is
blocked (cancel/deadline, successful/error return, no HTTP attempted). Suggestion
regressions assert exact non-execution text/raw argument spellings before/after
a new user turn with ordinary call_id-correlated tools. Existing HTTP 501/503 behavior is
unchanged. Source pins and shape choices: [Pi provenance](../provenance/pi-codex.md).

Validation: **PASS** `make check` (all unit/race tests, vet, pinned
staticcheck/govulncheck with no vulnerabilities, bounded existing decoder/tool/
safety fuzz smoke tests), `make build`, and `git diff --check`. The initial lint
run identified capitalized error strings; corrected and full checks rerun green.
After the independent-review suggestion/cancellation fixes, reran all three
commands successfully, plus `go test -race ./internal/provider/codex -count=20`.
Independent final review cleared both fixes with no new findings. Independent
validation also passed make check/build and repeated provider race tests. No
later-phase work or live verification was performed.

## Retained blocker / next steps

Live eligible catalog **NOT PASSED**: discovery omitted the replay-proven model;
one candidate follow-up failed before text proof. No eligibility expansion or
new live attempts occurred. Deterministic Phase 4 tests do not clear that gate.
Production credential lifecycle remains Phase 7. No public API-key fallback
was added. Independent review is complete for this checkpoint; Phase 5 owns the
parser and completion semantics, Phase 6 chat integration. Usage presently has
only input/output token totals; further fields should be added only with tested
parser/Atuin requirements, not speculative provider registries.
