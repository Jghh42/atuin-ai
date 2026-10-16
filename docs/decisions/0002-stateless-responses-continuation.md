# ADR 0002: Stateless Responses transcript replay

- Status: accepted invariants; isolated Codex replay passed on `gpt-6.1-sol`; production/SIWC validation separate
- Date: 2026-10-16

## Context

Atuin persists its own transcript and resends relevant history. Its session
UUID is not an OpenAI response ID. SIWC HTTP preview requirements mandate
`store: false`, `stream: true`, input arrays with required context, and omission
of `previous_response_id`. See the official
[preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)
and [completion rules](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)
retrieved 2026-10-16. Documentation research is not live compatibility proof.

## Backend decision update

[ADR 0004](0004-pi-style-codex-authentication.md) selects native Codex device
login/backend inference initially and defers SIWC. The lifecycle and full-history
invariants below still apply. SIWC-only field/tool restrictions are retained
for that future mode, not presumed to describe the Codex backend. Pin and
validate each provider's projection separately before enabling an intended model.

## Decision

One Atuin chat request produces one Responses generation. Replay all required
history on each generation; do not persist transcripts or maintain hidden
server-side sessions. Send policy via `instructions` or documented developer
messages, not explicit system message items. Always use `store: false` and
`stream: true` in the selected Codex mode and future SIWC mode; omit each
mode's unsupported fields.

Map assistant tool uses to `function_call` items and results to
`function_call_output`; retain the provider's **call_id**, not output-item ID,
for Atuin tool IDs and output correlation. Preserve user/assistant text and
all required tool exchanges in order. The CLI alone executes capability-gated
local tools and submits results in a subsequent request. No automatic
server-side tool loop or second generation within an Atuin request.

Buffer executable calls until `response.completed`, then validate the entire
batch before emission. Reject mixed `suggest_command`/ordinary-tool batches
before emitting any call. Unknown/hosted/server tools never reach the CLI.
Text may stream earlier; failure, incomplete, premature EOF, or malformed
calls are not success. Never automatically retry after downstream content.

The SIWC preview requires namespaced function/custom tools or `additional_tools`
input items. Exact declaration, returned names, and continuation item shape
must be captured by a safe live proof before projection is implemented.

## Alternatives considered

- `previous_response_id`-only continuation: unsupported by this HTTP flow and
  would couple Atuin sessions to provider state.
- Server transcript persistence: duplicates CLI ownership and adds resource,
  privacy, and recovery costs.
- Silently discarding opaque reasoning state: unsafe; may produce incorrect
  continuation or rejected requests.

## Consequences

Initially allow only models proven to complete full-history text and tool
replay without unavailable opaque state. If a model requires reasoning items
that Atuin cannot preserve, mark it unsupported; separately design bounded
encrypted provider state and a new ADR before enabling it. Do not infer that
one successful text response proves multi-turn support. The live proof must
continue a tool call in a new request without `previous_response_id`, verify
completion, and record whether additional state was needed. The isolated Codex
probe passed on `gpt-6.1-sol` without opaque reasoning items in the tested tool
response; see [live evidence](../proofs/codex-phase0.md). This does not prove other
models/transcripts, production integration or deferred SIWC. This ADR implements
no server behavior.
