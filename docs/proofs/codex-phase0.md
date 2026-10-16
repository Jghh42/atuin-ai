# Native Codex Phase 0 live evidence

- Date: 2026-10-16 (UTC)
- Environment: Linux amd64, Go 1.26.9, isolated native Go proof harness
- Decision: [ADR 0004](../decisions/0004-pi-style-codex-authentication.md)
- Model tested: `gpt-6.1-sol`
- Account context: user intended the target Business workspace; no account or
  workspace identifiers retained and no independent plan-entitlement claim made.

## Observed results

| Check | Result |
| --- | --- |
| Fresh Codex device authorization and code exchange | PASS |
| Isolated credential persistence, loaded by a new process | PASS; generation 1 |
| Streamed synthetic greeting with successful terminal snapshot | PASS |
| Harmless no-argument synthetic function call | PASS |
| New full-history request with correlated `function_call_output` | PASS |
| Continuation uses the synthetic output marker | PASS |
| Provider call ID used separately from output-item ID | PASS |
| Opaque reasoning items in tool response | None observed |
| Replay without `previous_response_id` or retained opaque provider state | PASS for this model/transcript only |
| Refresh rotation, durable replacement save and inference after refresh/restart | PASS; generation 1 → 2 |
| Local logout / remote revocation | NOT RUN; isolated credentials deliberately retained, remote revocation not claimed |

Each of two completed proof runs made three successful sequential inference
generations: greeting, one synthetic call and its continuation. The second run
used the replacement credential loaded after refresh. No model-selected
shell commands or file tools were executed. No Pi/Codex subprocess or existing
Pi/Codex credential file was used. Login and inference ran in separate processes,
so inference read the saved isolated credential record rather than login memory.
A subsequent explicit refresh succeeded once, without retrying the old token.
The probe required a distinct replacement refresh token and matching account,
then fsynced/atomically replaced the record. Separate new `status` and `prove`
processes loaded generation 2; the post-refresh proof passed text, tool and
full-history continuation again. This proves probe process restart/persistence,
not a production server lifecycle or power-loss test.

The first inference checks failed a probe MIME-header assumption. A subsequent
probe accepted recognizable SSE framing as Pi does, still bounded and requiring
an explicit completed snapshot matching the streamed text. The successful report
had all text/tool/continuation/correlation booleans true and
`reasoning_items_present: false`. Do not generalize that a nonstandard header
makes arbitrary non-SSE HTTP 200 responses successful. Regression tests cover
nonstandard media types, missing completion, mismatch and redaction.

The local port-1455 page only displayed the private device code and did not
auto-update; it was not an OAuth callback. Activation was completed in OpenAI's
browser flow. No codes, tokens, raw provider bodies, authorization URLs,
screenshots or diagnostic request IDs are retained in this report.

**The revised Phase 0 exit is cleared** for the selected Codex backend and
this intended model/transcript. Phase 1's bounded HTTP shell was already
verified. Production auth/provider integration remains for later phases; this
proof is not a completed server. SIWC remains separately deferred and unproven.
No logout or remote-revocation success is claimed; retained probe credentials
remain private and must not be imported into Pi/Codex or committed.

## Later retained-session recheck (separate evidence)

A separately authorized bounded retained-probe recheck on the same date passed
text, synthetic tools/full-history replay, exactly one durable refresh from
generation 2 to 3 and inference in a new process. It did not perform fresh login,
logout or discovery and is NOT production validation. See the
[sanitized recheck record](codex-retained-probe-2026-10-16.md); the historical
results above are not retroactively changed.
