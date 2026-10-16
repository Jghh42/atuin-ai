# Authentication: initial Codex path and deferred SIWC

## Current decision

[ADR 0004](decisions/0004-pi-style-codex-authentication.md) selects native
Pi-style Codex device login and direct Codex-backend inference for the initial
implementation. Independent SIWC browser login and public Responses inference
remain a future feature, not an initial release gate. This replaces the former
public-Responses-only rule; it is not merely a new login UI for the same token.

**Implementation status:** the phase 1 Atuin HTTP shell exists, but production
login and inference do not. The `cmd/siwc-proof` harness is an isolated SIWC
experiment, not a Codex device-login implementation. A separate isolated
[`cmd/codex-proof` probe](codex-proof.md) now implements the selected device flow;
its live outcomes must be recorded separately, not inferred from the code.

## Initial login design

The Go executable will request a device code using the reviewed Codex OAuth
registration, show the OpenAI activation page and user code privately, and poll
until approval, denial, expiry or cancellation. The user can complete approval
in a browser on another machine. No SSH callback tunnel is required. Exact
commands are to be finalized during implementation; do not assume the SIWC
harness or the phase 1 server already exposes them.

The server will own its credentials in a separate protected directory: owner-
only directories/files (`0700`/`0600`), atomic saves and one lifetime writer
lock. Never read/import existing Pi/Codex auth files, use browser cookies, or
share their refresh-token session. Disable credential/device-code logging and
never put tokens in process arguments or repository fixtures.

The source-informed inference route is
`https://chatgpt.com/backend-api/codex/responses`, with Bearer authentication
and the Codex account header. This is **not** independent SIWC or the public
`api.openai.com/v1/responses` route. Reference availability does not establish
OpenAI support for our integration; policy and protocol changes remain risks.
Atuin still owns all local-tool execution; no Codex/Pi subprocess is selected.

## Selected-backend Phase 0 validation

- [x] Pin the reference package/version and inspected artifact hashes and review
  registration, device/token endpoints, polling rules, headers and request/tool
  shapes. See [source provenance](provenance/pi-codex.md); an exact upstream Git
  commit is not asserted.
- [x] Obtain explicit approval for a fresh, isolated device login and small
  ChatGPT Business plan usage. Keep existing Pi/Codex sessions untouched.
- [x] Complete fresh isolated device login and save credentials; a separate
  process loaded the saved record. See [live evidence](proofs/codex-phase0.md).
- [x] Complete streamed text through an explicit successful terminal event on
  `gpt-6.1-sol`. Cancellation/error paths have fake-server/unit coverage; this
  is not a live cancellation test.
- [x] Complete a harmless synthetic function call and a second request with
  `function_call_output` and complete required history, correlated by `call_id`.
  Prove stateless replay; do not silently discard required reasoning state.
- [x] Prove locked refresh rotation, atomic durable replacement save and
  post-refresh inference in a new process (generation 2).
- [ ] Cleanup follow-up, not a cleared operation: probe credentials are retained
  privately. Remote revocation is not implemented/confirmed; never claim
  revocation from local deletion alone.
- [x] Retain only manually reviewed sanitized protocol shapes, outcomes and
  selected model. No tokens, codes, raw URLs, account identifiers or screenshots.

The revised Phase 0 exit is cleared by this project's own isolated Codex
probe, not by existing Pi usage. Production lifecycle/security hardening still
belongs to later phases. Cleanup remains an explicit follow-up, not evidence of
remote revocation. API-key inference is a separately billed fallback.

## SIWC investigation and future work

The independent browser/PKCE attempt reached agent naming and consent for basic
profile information and ChatGPT-plan usage, then OpenAI returned an organization
policy denial: `3p_delegated_access_policy_denied`. No successful token exchange,
model listing, inference, tool continuation or refresh was demonstrated. The
precise admin remedy and Business eligibility for this flow remain unresolved.
This result does not establish that local tools are unsupported: OpenAI's
[OSS guide](https://developers.openai.com/siwc/token-sharing-open-source) and
[desktop-app cookbook](https://developers.openai.com/cookbook/articles/sign-in-with-chatgpt)
explicitly describe locally hosted/personal projects and dynamic registration.

Retain the [deferred SIWC design/checklist](authentication-siwc-deferred.md),
[SIWC harness notes](siwc-proof.md), and
[superseded ADR 0001](decisions/0001-upstream-authentication.md). Revisit when the
target workspace's access is understood. SIWC must have its own credential and
request contract; Codex tokens, endpoints and namespace assumptions are not
interchangeable with SIWC. Do not silently switch existing users between them.
