# Retained native Codex probe recheck — 2026-10-16

Sanitized evidence supplied by the coordinator's separately authorized, bounded
existing-probe check. The documentation writer did not repeat the live check or
inspect credentials. This is a fresh check of the RETAINED ISOLATED PROBE session,
not fresh authorization and not the production credential manager/server.

## Reported observations

| Check | Result |
| --- | --- |
| Retained isolated access credential initially unexpired | PASS; local generation 2 |
| Completed streamed text | PASS |
| Synthetic function call | PASS |
| Full-history tool-result continuation | PASS |
| Exactly one refresh with durable replacement persistence | PASS; generation 2 → 3 |
| New process loaded replacement and completed streamed text | PASS |
| New-process synthetic call/full-history replay/call correlation | PASS; all reported booleans true |
| Total inference generations | Exactly 6 |
| Automatic retries | None |
| Fresh login / logout / catalog discovery | NOT RUN |
| Production login / production catalog / production lifecycle | NOT TESTED |

No tokens, account/workspace identifiers, private credential paths, pending
codes, raw provider bodies or terminal captures are retained here. No new model
eligibility claim is made from this report. There was no Pi/Codex credential
import, and the probe record must not be copied into production state.

## Gate interpretation

This supplements the [historical Phase 0 proof](codex-phase0.md): the retained
probe session again supported text, synthetic tools, full-history replay and
refresh/restart under the bounded authorized check. It does **not** validate the
new production store, manager, CLI login or server composition.

**Fresh production login/live lifecycle: NOT PASSED.**
**Live eligible production catalog: NOT PASSED.** No catalog was requested in
this recheck; the prior missing-eligible-model result and failed candidate probe
are not cleared. Security review and independent verification of Phase 7 were
still in progress at the original probe-evidence handoff; the subsequent code
checkpoint's re-reviews/verification cleared delivery, not these production
gates. No Phase 8 execution or production success is implied by PROBE evidence.
