# ADR 0001: Independent SIWC browser authentication; device login deferred

- Status: accepted design; live validation blocked
- Date: 2026-10-16

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

Use independently registered SIWC browser Authorization Code + PKCE S256 on
the same machine as the service, with a `127.0.0.1` HTTP callback. Start the
listener before opening the browser. Persist a stable `ext_agent_host_id`.
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
native device authorization. **Device login is deferred/unavailable for this
project unless OpenAI documents or confirms support for independent dynamic
clients.** This is a supported-path decision, not proof that no such endpoint
exists. Do not probe or borrow Codex client IDs, device endpoints, private
credential stores, cookies, or private ChatGPT backend routes.

Use only public `/v1/models` and `/v1/responses`. OAuth is a credential source
behind a narrow interface, not a dependency of Atuin protocol handling.
API-key development mode is separate and cannot satisfy SIWC exit gates.

## Alternatives considered

- Native device flow: preferred later if independently supported; not established.
- Codex app-server subprocess: optional future experiment, not selected; adds a
  binary and tool-orchestration boundary that must be reviewed separately.
- Remote VM credential transfer: documented upstream but unnecessary for local
  v1 and outside this decision's implementation scope.

## Consequences

Browser access and explicit user login/consent are prerequisites. A single
credential writer must serialize rotating refresh and atomically persist the
replacement token set with owner-only permissions. Full lifecycle engineering
belongs to Phase 7, not this documentation change. See the
[live proof checklist](../authentication.md). Phase 0 remains blocked until
independent login, models, streamed text, tool continuation, and refresh are
proven. No credentials or authorization URLs belong in evidence records.
