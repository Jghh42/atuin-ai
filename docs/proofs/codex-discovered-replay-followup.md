# Bounded discovered-candidate replay follow-up

Date: 2026-10-16. **Live eligible-catalog gate remains NOT PASSED.**

Coordinator authorized one bounded harmless candidate attempt, using only this
application's retained isolated Phase 0 credential through native Go. No login,
refresh, Pi/Codex credential access, cookies, subprocess inference, local tool
execution, or model guessing occurred.

The opt-in command was run once:

```sh
go run ./cmd/codex-proof prove-discovered --approve-live
```

The implementation fetches actual native discovery metadata, excludes hidden
models, prefers actual `-sol` candidates using upstream priority (no unverified
release-version assumptions), otherwise uses upstream ascending priority and
ID ties. It selects only one candidate and delegates to the existing bounded
proof: streamed exact greeting, one synthetic no-argument `echo_marker` call,
and full-history `function_call_output` continuation using `call_id`, without
`previous_response_id` or replaying opaque reasoning items. No returned tool is
executed. Failure aborts immediately; there are no retries or alternate models.

Safe report from the sole invocation:

```json
{"model":"","text_completed":false,"tool_completed":false,"continuation_completed":false,"call_id_correlated":false,"reasoning_items_present":false,"replay_without_opaque_state":false}
```

The command exited 1. No text proof completed; tool and continuation gates were
not reached/passed. This path can make at most one inference request before
such an early failure. The initial invocation printed only a generic failure,
not a finer failure classification, so its precise failure cause is **not
established**. Do not infer that the model lacks replay support or that the
credential is revoked. No repeated attempt was made merely to improve diagnostics.
The command now supports safe locally constructed diagnostic classifications
for a future separately authorized run, but no new live result is claimed.

The unproven candidate ID and raw catalog/request/response/account data were
not retained or printed. The production allowlist is unchanged:
`gpt-6.1-sol` only. It remains absent from the last successfully decoded native
catalog. Existing Phase 0 proof is historical evidence, not current discovery
availability or a pass for another model.

## Outcome

Stop live probing at this bounded failure. No model is enabled. Phase 4+ may
continue with deterministic injected providers; live eligibility and production
credential integration remain open gates. See
[Phase 3a status](../plans/phase3a-status.md) and
[original discovery evidence](codex-discovery-phase3a.md).
