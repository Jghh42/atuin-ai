# ADR 0001: SIWC browser authentication with loopback or SSH forwarding

- Status: superseded for initial implementation by [ADR 0004](0004-pi-style-codex-authentication.md); retained for deferred SIWC work
- Date: 2026-10-16

## Subsequent result and deferral

A later live probe reached agent naming and ChatGPT-plan consent, then returned
`3p_delegated_access_policy_denied` and an organization-access message. No
successful token exchange or inference was demonstrated. The user selected
native Pi-style Codex device login initially; independent SIWC remains desired
for later. The decision below records the earlier design, not active v1 rules.
See [the current authentication decision](0004-pi-style-codex-authentication.md).

## Context

The target is a personal ChatGPT Business seat using the public Responses API.
The implementation plan reports that this seat works through Codex/Pi; that is
not evidence that an independently registered SIWC client works. No account was
authorized and no live inference or refresh was performed for this record.

Official documentation was retrieved over HTTPS on 2026-10-16 (public-page
research only):

- [OSS overview](https://developers.openai.com/siwc/token-sharing-open-source)
- [Registration and sign-in](https://developers.openai.com/siwc/token-sharing-open-source/sign-in)
- [Models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)
- [Accounts, refresh and sign-out](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions)
- [Token reference](https://developers.openai.com/siwc/token-sharing-open-source/token-reference)
- [Errors and recovery](https://developers.openai.com/siwc/token-sharing-open-source/errors-and-recovery)
- [Preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)
- [Self-hosted VMs](https://developers.openai.com/siwc/token-sharing-open-source/self-hosted-vms)

These are mutable documentation, not pinned source or proof of account access.
The current preview specifically requires namespaced function/custom tools or
`additional_tools` input items. Do not assume ordinary flat API-key function
tool examples work here; exact tool wire shape remains a live proof gate.

## Decision

Use independently registered SIWC browser Authorization Code + PKCE S256 with
a `127.0.0.1` HTTP callback on the service machine. Start the listener before
opening the browser. On a remote development machine, the browser runs on the
user's workstation and reaches that callback through an explicit SSH local
port forward; the callback never binds to a non-loopback address. Persist a
stable `ext_agent_host_id`.
For first registration use `dynamic_agent_client` plus our actual consistent
`agent_name_hint`; exchange using the issued client ID returned by the
callback, never the registration entrypoint ID. Reauthorization uses that
issued ID. Only the callback port may vary between attempts; scheme, host and
path remain fixed, and the exact URI must match within each attempt.

Request `openid profile email offline_access resource.invoke
chatgpt.tokens.use.direct` and resource `https://api.openai.com/v1`. Validate
state and nonce, PKCE, callback identity, ID-token signature/JWKS, issuer,
audience, expiry, and returning-account identity. Use token-response granted
scopes, not callback hints, to authorize plan inference. No client secret or
partner API key is required by the documented direct flow.

The official independent-client flow reviewed documents browser/loopback, not
native device authorization. **V1 supports only browser Authorization Code +
PKCE.** Device login and credential-file import are deferred; they are not
release gates. This is a supported-path decision, not proof that no device
endpoint exists. Do not probe or borrow Codex client IDs, device endpoints,
private credential stores, cookies, or private ChatGPT backend routes.

Use only public `/v1/models` and `/v1/responses`. OAuth is a credential source
behind a narrow interface, not a dependency of Atuin protocol handling.
API-key development mode is separate and cannot satisfy SIWC exit gates.

## Alternatives considered

- Native device flow: not documented for independent SIWC clients and deferred.
- Codex app-server subprocess: not selected; it adds a binary and a conflicting
  tool-orchestration boundary.
- Credential transfer/import: documented for self-hosted VMs but deferred
  because SSH callback forwarding gives v1 one login mechanism.
- Public callback listener: rejected; remote development must use loopback plus
  SSH forwarding.

## Consequences

Browser access and explicit user login/consent are prerequisites. Remote
development additionally requires an SSH local port forward kept open through
the code exchange. A single credential writer must serialize rotating refresh
and atomically persist the replacement token set with owner-only permissions.
Full lifecycle engineering
belongs to the future SIWC lifecycle integration, not this historical record.
See the [deferred SIWC checklist](../authentication-siwc-deferred.md). The former
SIWC Phase 0 gate required independent login, models, streamed text, tool
continuation and refresh. It is now deferred by ADR 0004, not an active blocker
or a successful SIWC result. The revised Codex gate passed separately. No
credentials or authorization URLs belong in evidence records.
