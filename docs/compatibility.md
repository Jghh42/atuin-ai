# Compatibility baseline and fixture provenance

The pinned [source manifest](provenance/upstream-sources.json) and existing
[protocol specification](specs/atuin-ai-server-api.md) define the initial
client/server/core research baseline. These are exact source commits, not a
promise of support for a released Atuin version or all historical clients.
The implementation plan's narrower v1 choices govern the new design: IPv4
loopback only, no client Bearer auth, no server tools, one upstream generation
per Atuin request, full-history replay and rejection of mixed suggestion/tool
batches. Reference-server options described in the specification are not
features implemented here.

## Fixture inventory

[`testdata/compat/manifest.json`](../testdata/compat/manifest.json) records the
Phase 2–3 request fixtures, tool/prompt goldens and Go table-test provenance.
Cases are independently recreated from pinned core behavior or explicitly
original boundary/consistency tests. Prompt and tool-description prose is
newly authored, not imported. Unit/snapshot tests pass; this is **not** a real
CLI or inference proof.
The broader source inventory in implementation-plan section 8 remains a
candidate list, not a completed import.

For each later case, add a manifest entry with:

- stable `id` and repository-relative `destination`;
- `upstream_repository`, full `upstream_commit`, `original_file` and
  `original_test` (explicitly note if no named test exists);
- `license_spdx`, applicable copyright/notice reference and retained license;
- `reuse`: `copied`, `transformed`, or `independently_recreated`;
- `intentional_behavior_differences` (an explicit empty array when none);
- redaction/modification notes and reviewer verification of provenance.

For wholly original cases, use `reuse: original`, identify their specification
basis and explicitly mark upstream fields not applicable rather than inventing
a source. Do not label a source-informed recreation as original. Keep case
request/expected JSON/SSE separate from this inventory and record every case,
not merely its directory. Normalize volatile UUIDs, usage, JSON ordering and
text segmentation only when those are not the behavior under test.

## Phase 2 normalization and explicit policies

- String messages become text blocks; legacy assistant calls append after
  existing content. Empty string content contributes no block when merged
  with legacy calls. Unknown fields are ignored.
- Tool use belongs to assistant messages; results belong to user messages.
  IDs/names must be nonempty; call IDs are unique across the supplied history.
  Every ordinary call must have exactly one later result; orphan, duplicate,
  and unresolved exchanges are rejected instead of silently repaired.
  Correlation is by ID, not adjacency: parallel results and interleaved CLI
  assistant call groups are supported. `suggest_command` needs no result.
  These consistency checks intentionally strengthen the reference decoder.
- Remote references become `[Tool result no longer available]`. The declared
  length contributes to the estimate before hydration; missing/null lengths
  contribute zero. Negative or non-integer lengths are invalid; arithmetic
  saturates safely rather than overflowing.
- The estimate is floor(total grapheme-cluster characters / 4), rejecting values
  above 180,000. Tool input is re-encoded without insignificant whitespace or
  alternate string escapes for estimation, using `json.Number` for exact
  numeric precision. Numeric lexemes are retained (unlike the reference's
  runtime-dependent number normalization); original raw input is not changed.
  Context/config are not included, matching the reference transcript estimate.
- Model selection is limited to configured aliases, with unknown/malformed
  selections falling back to the configured default. Capability strings are
  whitespace-trimmed; unknown capabilities (including history v2) are ignored.
  `prompt_fn` is always cleared in standalone mode.
- User-context limits are the first **10 raw entries** before invalid entries
  are dropped; skills use the first **50 raw entries**. Truncation retains
  150/10,000 context characters plus `... (truncated due to length)`, and
  64/1,024 skill characters or 500 overflow characters plus `... `.
  Characters are grapheme clusters, including combining marks and emoji.
- Session IDs accept the canonical hyphenated 36-character UUID form
  (case-insensitive, normalized to lowercase). Missing/null/empty creates a
  random UUIDv7 with a millisecond timestamp. Versions other than v7 remain
  valid when supplied by the client. Malformed non-string session IDs reject.
  Invocation IDs are lenient optional strings, not UUIDs.
- In addition to the 4 MiB body limit, decoding caps JSON nesting at 64,
  structural punctuation at 65,536, messages at 4,096 and normalized content
  blocks at 16,384. These deliberate bounded-work restrictions reject with
  `invalid_request`; ordinary CLI histories fit well below them.

## Phase 3 tools, prompts and safety policies

- The model sees `suggest_command` first, followed by enabled v1 tools in order:
  `read_file`, `edit_file`, `write_file`, `atuin_history`, `atuin_output`,
  `execute_shell_command`, `load_skill`. History v2 and unknown capabilities do
  not enable tools. There are no server-side tools or execution hooks.
- Schemas retain the specified required/optional fields and bounds, allow
  unknown additional properties, and do not apply public API strict-mode rules
  to Codex. Defaults remain explanatory prose, not server-inserted values.
  In particular, `atuin_output` still advertises the core's `[[0,1000]]` default,
  but its description warns that this CLI fetches all output when ranges are
  absent/empty. Prefer explicit ranges.
- Prompt and tool-description prose is original, not byte-compatible with core.
  Dynamic client strings are escaped and labeled as untrusted data. Stable
  environment and user/skill metadata are separated from volatile UTC date,
  directory and last-command context. Environment retained-prefix limits are
  30/40/20/50 graphemes (OS/distro/shell/language); turn limits are 60/300
  (directory/last command), followed by `...` when truncated.
- `tools.ValidateBatch` accepts only advertised tools, nonblank unique call IDs,
  complete object arguments, and valid field types/enums/bounds. Parallel local
  calls preserve their order. Exactly one suggestion may form a terminal batch;
  a suggestion mixed with anything else rejects the entire batch. Errors return
  no partial batch and include no tool text. The future turn coordinator must
  call this only after explicit provider success and emit only its result.
- Tool arguments reject duplicate keys at any depth, invalid UTF-8/unpaired
  Unicode surrogate escapes, and numbers that the pinned Rust JSON decoder
  cannot represent (including unknown properties). Integer fields require
  integer JSON spellings rather than fractions/exponents that the CLI would
  default or reject. Range endpoints fit signed 64-bit integers; offsets may
  additionally use unsigned 64-bit positive values, with negative offsets
  retaining the CLI's default-to-zero semantics. Raw numeric lexemes are not
  rounded or rewritten. Other unknown valid JSON fields are retained.
- Arguments are capped at 256 KiB per call, 32 calls per batch, recursive value
  depth 64 and 65,536 argument-parser work tokens. Rejection is atomic. An
  escalated suggestion must also fit the argument cap.
- The danger backstop can only raise a suggestion to `high`, never lower its
  supplied danger or delete its notes. It appends fixed warning messages for
  recognized recursive deletion, privileged execution, disk writes, filesystem
  destruction, world-writable permissions, download-to-shell and fork bombs.
  It deliberately removes core's keyword-prefilter blind spots, catches
  non-root recursive deletion and does not skip scripts merely because they
  start with a comment. Matching quoted text may cause false positives; aliases,
  obfuscation, expansion and other shell/language constructs can evade it.
  **No match is not proof of safety.** This heuristic is not a parser, sandbox
  or authorization mechanism; CLI permissions remain the enforcement point.

## Phase 4 Responses history exception

Ordinary historical local tools retain original call_id/raw argument JSON and
must have corresponding results across the supplied history. No missing-result
repair or executed output is fabricated. Atuin's `suggest_command` is a terminal
suggestion with no result, so it instead becomes deterministic assistant text:
`Terminal command suggestion (suggest_command; suggestion only, not evidence of execution). Original arguments JSON: `
followed by unchanged raw argument JSON. It is never replayed as a dangling
Responses function call. This explicit compatibility exception preserves the
suggestion without asserting that the user ran it. It differs from pinned Pi's
`transform-messages.js` synthetic-result repair; [provenance/pins](provenance/pi-codex.md)
record the inspected dependency and decision. `internal/provider/codex/request_test.go`
contains independently written synthetic before/after-user-turn regressions,
including raw numeric spellings and ordinary paired local calls. This is not
live proof or real-CLI validation; the live eligible-catalog gate remains NOT PASSED.

Before importing, follow the [third-party process](provenance/README.md).
Before claiming model support, complete the selected-backend
[live validation](authentication.md). The isolated Codex proof covers only the
model/transcript documented in [its report](proofs/codex-phase0.md), not real
Atuin CLI compatibility or independent SIWC.
No live data or credential artifacts may become compatibility fixtures.

## Phase 8 actual pinned CLI / original-source oracle

[Phase 8 status and exact commands](plans/phase8-status.md) distinguish local
synthetic passes from production/live/release gates. The unmodified pinned
Atuin now runs under stdlib PTY automation against the composed native Go
application with fake OAuth/discovery/Responses. Initial review was NOT CLEARED:
HOME/XDG alone did not confine ancestor context discovery/embedded commands.
No retrospective absence of reads/execution or incident is inferred. Corrected
invocations run in an fd-pinned Bubblewrap filesystem/PID namespace (host
ancestors/HOME not mounted), with fixture metadata/symlink checks before EACH
launch, isolated HOME/ATUIN_HOME/XDG/database/sandbox paths; OSS and no client
authentication are asserted. All seven tools are exercised by the actual CLI,
including positive captured-output retrieval through an isolated real daemon.
Permissions, shell-hook insert versus execute, parallel correlation, full
history/session reuse and process restart, revoked/expired credentials,
failed/unfinished tool calls and cancellation have client-observable assertions.
These are synthetic backend proofs, never live account or catalog evidence.

The source-built original reference server/core and fake Chat Completions
sidecar run only on an internal Docker bridge, with **no published ports**.
Identical fixture requests are compared with native Go/fake Responses. Both
fakes first validate outbound model, exact ordered schema declarations and
complete ordered history/call/result correlation using an independent test
oracle, so canned answers cannot hide dropped projection data. UUID shape,
header/done consistency and explicit continuation identity are validated BEFORE
normalizing valid session UUIDs, usage, JSON order and adjacent text segments. Mixed-batch rejection, enhanced sudo risk and unknown-null versus
reference-zero token usage (and reference `"nil"` cost strings) are explicitly
asserted differences, not silently erased.
Exact positive events and one-generation counts compare. Per-case source,
license and derivation: [`phase8-manifest.json`](../testdata/compat/phase8-manifest.json);
[toolchain/image/artifact provenance](provenance/phase8-tooling.md).

Default `make check` includes deterministic corpus/manifest tests but skips
external gates when artifact manifests are absent. `make compatibility` fails
without them; a default skip or a green Go-only CI is NOT an external-gate pass.
The required-capability PR/push/manual workflow (NOT pull_request_target) has
not yet run for these changes. Its aggregate depends on both Go and actual
compatibility; administrators must require that check externally. Interrupted
creation / global Go timeout cleanup uses a preregistered run/lease watchdog
and workflow-always cleanup, scoped only to owned labels.
No production runtime compatibility policy was relaxed or changed in Phase 8.
