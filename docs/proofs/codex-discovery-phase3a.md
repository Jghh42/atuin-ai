# Phase 3a redacted native discovery evidence

Date: 2026-10-16. **Eligible-catalog live gate: NOT PASSED.**

The native Go opt-in command `go run ./cmd/codex-proof discover --approve-live`
used only this application's existing isolated Phase 0 record under
`~/.atuin-ai-codex-proof/`, opened through the probe's locked, owner-only store.
Safe status established a current access token. No refresh or interactive login
was necessary or attempted. No Pi/Codex credential store, cookie, subprocess,
external login session or device code was accessed. Production does not load
this probe store automatically.

Pinned [source contract](../provenance/codex-discovery.md): native GET
`https://chatgpt.com/backend-api/codex/models?client_version=0.134.0`, Bearer
access token, matching `ChatGPT-Account-Id`, `Accept: application/json`, and
application-specific `User-Agent: atuin-ai/phase3a`. No raw request/response logs,
account identifiers, unknown model names or tokens were retained.

The final single-request probe emitted only:

```json
{"schema_validated":true,"http_status":200,"model_count":1,"proven_model_present":false,"proven_model_listed":false,"integer_priorities":true}
{"discovery_passed":false,"category":"invalid_or_no_eligible_metadata"}
```

The required native field/type subset decoded successfully, including integer
priorities. **Review correction:** at the time of this recorded probe,
`schema_validated` meant only this field/type decoding, not full metadata
validation of visibility enums, ID syntax, string bounds and duplicate IDs.
Do not reinterpret its historical `true` as proof of those additional checks.
Those checks are now shared by discovery and selection; malformed-metadata
report regressions pass, but no new live probe was run after this correction.
However **the only replay-proven model `gpt-6.1-sol` was absent**.
Thus the eligible snapshot could not be constructed, and the command exited 1.
Earlier bounded exploratory requests yielded the same counters. No inference
was attempted on the newly discovered, unproven model. Nothing was added to the
compatibility allowlist. Phase 0's historical inference/tool replay result is
not retracted or misrepresented as current catalog availability.

## Consequences and remaining gates

- Source/native-route/required-field decoding and authenticated HTTP discovery
  are established for this query/account at that time. Full metadata validation
  was not separately established by that historical probe. This is not discovery-to-eligible-model
  success, ETag proof, a release-order convention, or production readiness.
- Do not restore `gpt-6.1-sol` from a bundled/static fallback just because it
  worked in Phase 0, and do not enable another model merely because listed.
- The live exit needs either upstream availability of a replay-proven visible
  model or a separately reviewed Phase 0-style text/tool/full-history replay
  proof before expanding eligibility. User approval/policy decisions remain
  external to this bounded Phase 3a change.
- Phases 4+ can proceed using deterministic injected providers, credentials and
  HTTP; live integration/release remains blocked by this eligibility gate and
  future production OAuth/refresh/logout integration in Phase 7.
- Credentials remain in the isolated private probe store. No new credential was
  acquired or replaced. Local logout/remote revocation are not claimed.
