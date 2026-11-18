# Phase 5 checkpoint — native Responses SSE provider

2026-10-16. Independently reviewed implementation checkpoint, cleared for
user-authorized direct-main delivery. Exact Phase 5 SHA and CI outcome belong
in the delivery report, not inside the commit being identified.
Base Phase 4 SHA `2b446ac047d4a6acbb1f41b635dcc4625491d5ea`:
exact-HEAD GitHub Actions run `38953006689` completed **success** (read-only
verification before Phase 5 validation). This is Phase 4 evidence, not a claim
of exact-HEAD Phase 5 CI.

## Interfaces and deterministic policy

- `codex.RequestClient` now implements `provider.Provider.Stream`, using exactly
  one `Open` and no retries. No HTTP chat/turn-runner integration, production
  credentials/store, API-key fallback, model eligibility change, or live call.
- Only a delimited `response.completed` with `response.status:completed` and an
  output array succeeds. It is terminal: stop parsing and close immediately,
  without waiting for EOF or inspecting trailing events. EOF, `[DONE]` before
  completion, failed/incomplete/error, invalid JSON/UTF-8, truncated frames,
  mismatched event names, inconsistent final snapshots and invalid calls fail.
  Errors do not contain provider payloads/URLs/tokens. Context cancellation and
  deadlines remain `errors.Is` recognizable through credentials, Open and reads.
- Lifecycle snapshots (`created`, `queued`, `in_progress`, `completed`) require
  a nonempty valid `response.id`; a terminal-only completed snapshot is allowed.
  Every observed response ID must agree, including optional top-level
  `response_id` and IDs on otherwise additive events. Missing/null lifecycle
  IDs, malformed IDs, and conflicts fail before calls; no ID is invented.
  Non-lifecycle events may omit identity (missing/null), but any non-null ID
  they supply must be valid and agree. Nonterminal lifecycle snapshots may have
  absent output or an empty array only. Any nonempty lifecycle output fails
  closed as unsupported until real lifecycle-snapshot reconciliation exists;
  reasoning, hosted execution, refusal, text and function content are never
  silently dropped. Explicit null or non-array output is malformed and fails.
  Terminal-only completed snapshots remain supported.
- Every event is checked before struct decoding for duplicate JSON members at
  all depths, including ignored metadata, snapshots and usage. Case-fold aliases
  are also rejected in envelopes because Go struct decoding treats them as the
  same field. Unicode checks reject invalid UTF-8 and unpaired surrogate escapes
  in all keys/strings rather than allowing replacement by Go. Work is capped at
  65,536 JSON tokens and depth 64 (root depth zero), preserving number lexemes.
  Completed argument JSON is independently checked too (case-sensitive key
  uniqueness), including escapes nested inside the event's argument string.
  Split argument-delta surrogate pairs are allowed if the final JSON is valid.
- LF/CRLF, comments, blank heartbeats, unknown SSE fields, multiline data and
  arbitrary byte/UTF-8 fragmentation work. Every wire byte in a frame counts
  toward 1 MiB, including comments/unknown fields/line endings/delimiter. An
  overlong single line or an endless undelimited comment block fails. Ordinary
  separated heartbeat frames do not accumulate. No Scanner limit. Nonstandard
  MIME is accepted only if actual bounded recognizable SSE completes, matching
  the Phase 0 proof; arbitrary HTTP 200 is never success.
- Text streams immediately, retaining only bounded per-content SHA-256/length
  state, not accumulated text. Final text must match streamed bytes; a
  snapshot-only text part is emitted once. At most 128 output items and 128
  total text parts; indices must be in 0..127. Final output must reconcile every
  observed slot, checking each index against its final array bounds rather than
  comparing map cardinality. Empty content-part announcements also register
  bounded state and must appear in the final snapshot; duplicate announcements
  are idempotent. This explicit resource policy also applies to message-only
  streams, not just calls. Final text snapshots still obey the event-size cap.
- Function-call item ID and `call_id` stay distinct. Deltas require an existing
  indexed slot and matching `item_id`. Interleaved calls are reconciled in
  output-index order. Final arguments may extend a delta prefix, never replace
  conflicting bytes; repeated identical final snapshots/added items are
  idempotent. Deltas after finalization fail. Final arguments must be a valid
  JSON object, <=256 KiB per call; <=32 calls. IDs are nonempty printable ASCII
  <=256 bytes, names ASCII alphanumeric/underscore/hyphen <=64 bytes; duplicate
  call/item IDs and conflicting metadata fail. No missing names/IDs are invented.
- Sequence numbers, when supplied, must be nonnegative and increasing. Only an
  exact adjacent retransmission of the latest numbered event is ignored;
  conflicting/reordered numbered events fail with constant deduplication memory.
  Without numbers, deltas are distinct (identical text fragments are legitimate);
  snapshot/added-event reconciliation remains idempotent. JSON whitespace
  differences on same-sequence retransmissions intentionally fail closed.
- Calls are held until the entire completed output and usage validate, then at
  most one `provider.ToolCalls` batch is emitted. This is not CLI authorization:
  Phase 6 **must** call `tools.ValidateBatch` for registry/capability/schema,
  mixed-suggestion and danger policy before emitting any Atuin tool call.
  Unknown lexical function names are not executable without that gate.
- Refusal events/content fail with safe `ErrRefusal`; refusal prose is not
  forwarded as text and no calls escape. Previously streamed ordinary text
  cannot be retracted; Phase 6 must issue its one terminal error, not `done`.
  Unknown additive metadata events are ignored. Unknown tool/function/output
  item/content-part channels, all reasoning channels, any `*_call` event family,
  associated unsupported execution/approval-family channels, and
  non-message/non-function output items fail closed (`ErrUnsupported`),
  including provider-hosted web/file search, code interpreter, image generation,
  computer/local-shell/shell/apply-patch/MCP/custom calls and non-null namespaces.
  Execution-channel rejection does not depend on an eventual output snapshot
  containing that item. Opaque reasoning is **not** discarded or
  persisted: any reasoning output is unsupported under the present replay proof.
  This does not enable unproven models or invent a backend-state design.
- Generation (300s) and upstream inactivity (60s) start before Open, covering
  credential/header stalls too. Any received body bytes reset inactivity,
  including heartbeats; heartbeats cannot extend the generation bound. A joined
  watchdog plus cancellation-triggered Body.Close interrupts blocked reads.
  Sink failure cancels and closes immediately; all timers/watchdog work are
  stopped/joined. Transports must honor the standard response-body concurrent
  Close/unblock-Read contract, and credential sources must honor context. Sink
  functions must return from cancellation/write failure; Go cannot preempt an
  arbitrary blocking callback. No detached reader goroutine is used.
- `provider.Usage` now has input/output/total/cached token counts and individual
  Known flags. Input includes cache reads (not subtracted). Missing/null is
  unknown, explicit zero is known. Total is derived only if input and output
  are known; an explicit total must agree. Non-integer, negative, int64 overflow,
  sum overflow, cached>input, or known components>total fail. Cache is
  `input_tokens_details.cached_tokens`. No price/credit/cost claims.

## Tests and provenance

Original synthetic fixture and table/fuzz cases: `testdata/responses/README.md`.
Tests cover every split boundary and byte-at-a-time Unicode, LF/CRLF,
multiline/comments, interleaving/IDs, prefix/final reconciliation, duplicates,
usage, failed/incomplete/refusal/unknown/unsupported paths, bounded long lines,
comments/events/arguments/call count, malformed/truncated input, and sink errors.
Real TCP tests verify body/header stalls, activity vs generation deadlines,
nonstandard MIME, cancellation, sink failure, early close after completion,
failed responses/EOF, and exactly one request. Repeated fake blocked-reader
cycles verify cancellation closes/unblocks body; stage tests preserve context
errors. `FuzzResponsesSSE` is included in bounded `make check` and nightly CI.

Validation: **PASS** `make check` (unit/race tests, vet, pinned staticcheck,
govulncheck with no vulnerabilities, and four bounded fuzz smoke targets),
`make build`, `git diff --check`, and
`go test -race ./internal/provider/codex -count=20`. An initial staticcheck
capitalized error-string finding was corrected before rerunning full checks.
These are local validation results, not a claim of new exact-HEAD CI.

## Independent-review fixes (cleared)

Independent initial validation passed full check/build/race repetitions, but
review identified six parser gaps. Fixed with regressions in
`internal/provider/codex/parser_review_test.go`:

1. Sparse output/content indices cannot disappear from a shorter final array;
   checks run before processing completion snapshots and on output-item done.
2. Every unsupported `*_call` execution channel fails even if omitted from the
   final output; additive metadata compatibility remains tested.
3. Response identity is tracked and required on lifecycle/completed snapshots,
   with terminal-only, missing, malformed and conflicting-ID cases covered.
4. Duplicate JSON members (including struct case-fold aliases) are rejected
   before decoding, with bounded depth/token work and preserved numeric lexemes.
5. Unpaired surrogate escapes fail rather than being repaired, including in
   argument JSON. Valid pairs and fragmented argument escapes remain supported.
6. Empty content-part announcements register bounded, reconcilable state;
   negative/out-of-range/omitted parts and global announcement budgets are tested.

Shared seam: `internal/jsoncheck.ValidUnicode` is the existing tool gate's
project-authored Unicode checker extracted unchanged in behavior for actual use
by tools and the Responses parser. Tool schema/CLI policy stays unchanged;
parser-local `validStreamJSON` adds envelope duplicate/depth/work validation.
Final re-review identified one further gap: lifecycle snapshots checked identity
but ignored output. Nonempty `created`/`in_progress`/`queued` output now fails
closed, with per-event reasoning/hosted/refusal/function/text regressions and
malformed-output-type tests. Absent/empty lifecycle output and terminal-only
completion remain valid. This is a deliberately minimal policy, not partial
lifecycle-output reconciliation.

The stale RequestClient-not-a-Provider comment is also corrected. No later-phase
integration or live eligibility expansion is included. Post-fix `make check`,
`make build`, `git diff --check`, and provider race stress (`-count=20`) all
passed after the lifecycle-output fix too. The first full post-lifecycle check
stopped in the existing decoder `FuzzDecode` smoke target with `context deadline
exceeded`; an unchanged full rerun passed all targets, then build/diff check and
provider race repetitions passed. No unrelated decoder code was changed.
Independent final review cleared the original six fixes/shared helper and the
narrow lifecycle-output fix. Independent verification passed full check/build,
focused race x10, targeted lifecycle/regression tests and five isolated decoder
fuzz runs. The initial decoder fuzz deadline remains nonreproduced with cause
**unknown**, not classified as infrastructure; exact failure/pass context is
preserved in [autonomous progress](autonomous-progress.md). No timeout changes,
suppressions or decoder workarounds were added.

## Retained blockers

Live eligible-catalog gate remains **NOT PASSED**. No new live providers or
probe credentials used. Chat remains 501 with a valid injected catalog and
production 503 until later integration. Phase 6 owns the turn lifecycle and
CLI gate; Phase 7 owns credential persistence. Independent review is complete
for this checkpoint. No Phase 6 or later-phase work is included.
