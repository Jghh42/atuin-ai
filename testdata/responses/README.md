# Responses fixtures (Phases 5–6)

`phase5-text.sse` and the generated frames in
`internal/provider/codex/{parser,parser_review,stream}_test.go` are independently authored,
safe synthetic project data (Apache-2.0), not recorded provider/account traffic.
IDs, text, numbers and tool arguments are invented; no tool is executed.
The final blank line in `phase5-text.sse` is its required SSE frame delimiter;
`.gitattributes` exempts that specific fixture from `blank-at-eof` checks only.

Protocol references: Pi AI 1.1.0 pinned
`dist/api/openai-responses-shared.js`, `processResponsesStream`, SHA-256
`8744ad2ce9ce2512993360600760f5bcdbeb520d0a641c655b2a0b7cbe9db10a`,
and the independently implemented native Phase 0 proof. Package provenance,
MIT attribution (Copyright (c) 2025 Mario Zechner), license and exact pins:
[Pi provenance](../../docs/provenance/pi-codex.md). No upstream fixtures or
implementation text copied/transformed; no exact matching Git commit has been
established for the installed npm package and none is inferred.

Intentional differences from Pi: only explicit completed status succeeds;
refusal and all reasoning are fail-closed, no opaque-state persistence or
partial/incomplete success, no custom/provider-hosted tool support, no tool
execution, no retries, bounded framework bytes/slots/calls, exact snapshot
reconciliation, response identity, duplicate-member/Unicode validation and strict
integer token usage. Synthetic response IDs were added after review to exercise
the explicit lifecycle/completion identity policy; no real IDs are used.
Lifecycle-output regressions use matching synthetic IDs: nonempty output fails
closed, absent/empty output remains valid, and null/non-array output is rejected. Unknown additive metadata is
ignored, but unknown execution/state channels fail closed. See the
[Phase 5 policy](../../docs/plans/phase5-status.md).

Phase 6 sequence goldens in `internal/turn/runner_test.go`, exact SSE frame golden
in `internal/httpserver/stream_test.go`, and generated fake Responses snapshots
there, plus coupled TCP cancellation frames in
`internal/httpserver/native_cancel_test.go`, are independently authored
Apache-2.0 synthetic data. They exercise the
pinned Atuin protocol specification, not copied upstream test code or live
traffic. Terminal mixed batches intentionally fail atomically rather than
reproducing reference precedence; unknown usage is null, not fabricated zero.
No new third-party material or NOTICE change is required.
