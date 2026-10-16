# ADR 0004: Native Pi-style Codex login and inference initially; SIWC later

- Status: accepted — explicitly selected by the user
- Date: 2026-10-16
- Supersedes: ADR 0001 for the initial authentication and upstream route
- Retains: ADR 0002 transcript/tool ownership and terminal-event invariants

## Context and observed evidence

The independent SIWC probe reached agent naming and the consent screen for
basic profile information and ChatGPT-plan usage. Completion was denied with
`3p_delegated_access_policy_denied` and an organization-access message. No
successful OAuth exchange, inference, tool replay or refresh was demonstrated.
Screenshots, account identifiers, authorization URLs and diagnostic request IDs
are not retained here. The public docs do describe local OSS dynamic
registration, but no documented admin action for this tenant's denial was found.
This is not evidence that SIWC cannot support local tools in general.

The user reports that Codex device login works with the target Business seat
through Pi and is enabled in the admin panel. Installed Pi 1.1.0 source was
inspected without reading saved credentials. Its `openai-codex` provider:

- implements OAuth/device polling itself, with a fixed Codex client registration;
- exchanges device authorization results through OpenAI's OAuth token endpoint;
- stores/refreshes its own access and rotating refresh tokens;
- sends Responses-format requests directly to the ChatGPT Codex backend; and
- owns its own tools, without launching Codex CLI or app-server.

The user selected this approach for the initial implementation, with independent
SIWC integration deferred rather than abandoned.

## Decision

Implement the Pi-style Codex compatibility path natively in Go. Device-code
login is the initial login method, including on SSH development hosts; it does
not require a loopback callback or an SSH callback tunnel. The user opens the
OpenAI activation page in their own browser and explicitly authorizes the app.

The source-informed protocol baseline is:

- request device authorization at
  `https://auth.openai.com/api/accounts/deviceauth/usercode` using the fixed
  Codex OAuth registration found in the pinned reference;
- display the user code privately and activation page
  `https://auth.openai.com/codex/device`, then poll
  `https://auth.openai.com/api/accounts/deviceauth/token`;
- exchange the returned authorization code and verifier at
  `https://auth.openai.com/oauth/token` with the matching client registration
  and device redirect URI; refresh through that OAuth endpoint;
- send Responses-format streaming inference to
  `https://chatgpt.com/backend-api/codex/responses`, including Bearer
  authentication and the account identifier required by that protocol.

These are reference protocol facts, not commands to run yet. Pin and review the
exact client/endpoint/header/request contract before live validation. Identify
our integration honestly; do not claim to be Pi or an independently registered
SIWC app. Public availability of reference code is not an OpenAI support or
compatibility guarantee for this integration.

This explicitly replaces the initial public-Responses-only requirement and
permits the fixed Codex registration and Codex backend **only inside the
reviewed compatibility adapter**. It does not authorize arbitrary private
endpoints, cookies, credential scraping or impersonating another user's account.
No Pi/Codex process or agent loop is embedded. Own credentials are obtained by
fresh explicit login and stored separately from Pi and Codex. Never import,
read, modify or race-refresh their credential files.

Keep OAuth as a credential source and upstream protocol projection behind
narrow interfaces. Atuin remains the sole executor of capability-gated local
tools. One Atuin request still means one model generation; executable calls
remain buffered until successful provider completion. Required reasoning-state
replay, exact tool shape, model access, cancellation and bounded resources must
be proven; Pi's success does not prove our implementation.

API-key mode remains an optional, separately billed public Responses fallback.
It is not subscription-login validation.

## Consequences and risks

- Removes the currently denied independent SIWC registration from v1's critical
  path, but adds dependency on a source-informed Codex compatibility protocol.
- Codex registration policy, endpoints, headers, models and wire behavior may
  change or be restricted. Fail clearly; never fall back to cookies, another
  app's credentials or a different billing path without user approval.
- The Go binary remains standalone; no Node, Pi or Codex runtime dependency.
- Device polling must handle server intervals, pending/slow-down responses,
  expiry, denial and cancellation. Refresh rotation must be serialized and
  durably saved. All tokens and device artifacts remain secret.
- At acceptance the production server had no login or inference. Phase 7 now
  implements the lifecycle/composition as an independently reviewed code
  checkpoint cleared for delivery; fresh production live login/lifecycle
  remains NOT PASSED. This ADR alone is
  neither implementation nor successful live proof.
- Subsequent isolated probe validation passed fresh device login, streamed
  text, harmless tool continuation and refresh/restart on `gpt-6.1-sol`; see
  [live evidence](../proofs/codex-phase0.md). That clears the revised Phase 0
  exit, not production integration. Credentials are retained privately; cleanup
  and remote revocation are not claimed. Existing Pi login is not the evidence.

## Deferred SIWC work (retain as a future feature)

Implement independently registered browser/PKCE SIWC and public
`https://api.openai.com/v1/responses` later when the target account/workspace
policy and eligibility are understood and validated. Preserve ADR 0001 and
`docs/siwc-proof.md` as research/proof material, not active v1 requirements.
The existing harness is SIWC-only; do not feed it Codex tokens or treat it as
production credential storage. Revisit direct/SSH-forwarded loopback login,
registration, OIDC verification, granted scopes and the SIWC-specific request
restrictions as their own reviewed integration. No date or unsupported device
extension is promised.

## Reference source snapshot

Installed `@earendil-works/pi-ai` 1.1.0, MIT; source inspected only, not copied
by this ADR. Public source repository: https://github.com/earendil-works/pi
(`packages/ai/src/auth/oauth/openai-codex.ts`,
`packages/ai/src/providers/openai-codex.ts`,
`packages/ai/src/api/openai-codex-responses.ts`). A future implementation must
pin an upstream commit and preserve notices before importing code or fixtures.
The installed JavaScript artifacts inspected have SHA-256:

- `dist/auth/oauth/openai-codex.js`:
  `0b94a219ddb5982f0ee543ef03138ed0254ce6ee0a57c9d89d39c242e2eaf6f6`
- `dist/api/openai-codex-responses.js`:
  `b185956e11a5c5cb522e5cad77daf21ce31c3c304e86a8333ffcb49ff78466ea`
