# Authentication research and Phase 0 live proof

## Status

**Phase 0 exit: BLOCKED — user login and live SIWC proof required.** No SIWC
registration, OAuth exchange, model listing, inference, tool continuation, or
refresh rotation was run. No intended model is yet validated. Existing
Codex/Pi subscription login is not independent-client evidence. Public official
documentation was retrieved over HTTPS on 2026-10-16; see
[ADR 0001](decisions/0001-upstream-authentication.md) for references and the
browser/device decision, and [ADR 0002](decisions/0002-stateless-responses-continuation.md)
for replay requirements.

This change is documentation/provenance only. Go module/tooling are deferred
by task scope. No auth helper is shipped: safely validating OIDC and rotating
credentials would exceed a simple supported proof script, and belongs in a
reviewed harness or Phase 7 implementation.

## Prerequisites (all must be satisfied before the opt-in proof)

- [ ] User explicitly approves login, requested permissions, use of the target
  Business workspace/seat, and any plan usage. Confirm workspace policy permits
  this integration. Never infer eligibility from another application's login.
- [ ] A local interactive system browser and same-machine callback listener
  are available; outbound HTTPS to `auth.openai.com` and `api.openai.com` works.
  Do not use a remote harness callback assuming it reaches the user's browser.
- [ ] Re-read current official sign-in, models/inference, accounts/sessions,
  token, errors, and preview docs linked in ADR 0001. Record any changes.
- [ ] Use an independently registered app with its actual stable app name and
  stable host ID. Do not reuse Codex/Pi client IDs or credentials.
- [ ] Have a reviewed OAuth/OIDC client or controlled harness that validates
  signature via published JWKS, issuer, issued-client audience, expiry, nonce,
  state and PKCE S256. Discovery is at
  `https://auth.openai.com/.well-known/openid-configuration`.
- [ ] Use an isolated, owner-only local state directory outside the checkout
  (`0700`, files `0600`), atomic credential writes, a single writer and a clean
  sandbox. Disable shell tracing, HTTP debug logs, browser callback/access logs,
  history capture, telemetry and automatic retry. Keep secrets out of arguments.
- [ ] Agree on a harmless synthetic local function (no file/shell access) and
  current documented SIWC tool declaration shape. The preview says namespace
  groups or `additional_tools`, not assumed flat function declarations.

## Opt-in execution checklist

1. **Register/sign in.** Start `http://127.0.0.1:<available-port>/auth/callback`
   before opening the browser. Use fresh state, nonce and PKCE verifier;
   initial client ID `dynamic_agent_client`, actual `agent_name_hint`, stable
   `ext_agent_host_id`, resource `https://api.openai.com/v1`, and scopes
   `openid profile email offline_access resource.invoke chatgpt.tokens.use.direct`.
   Use the sign-in endpoint documented in ADR 0001. Never record the generated
   authorization URL. Validate state even for denial. Require the callback's
   issued client ID on registration; exchange the code using that ID, exact
   callback URI, verifier and resource. No secret. Validate the ID token and
   token-response granted scopes before inference. Refuse missing direct-use
   permission; a valid identity alone is insufficient. Keep credentials local.
2. **Capture catalog shape.** With this OAuth access token, request
   `GET https://api.openai.com/v1/models`. Record only field names/types,
   count/order semantics and selected non-sensitive model slug. Current docs
   show a `models` array with `slug`, `display_name`, `visibility`, filtering
   `visibility == "list"` while preserving order; do not assume API-key
   `data[]` shape. Select a model actually returned for this workspace.
3. **Proof A: text.** Send `POST https://api.openai.com/v1/responses` with the
   OAuth Bearer token, selected model, an input array containing a harmless
   synthetic greeting, `store: false`, `stream: true`. Consume text delta events
   through explicit `response.completed`. Failed/incomplete/error/premature EOF
   fails the proof, even if text appeared. No private backend or Chat Completions.
4. **Proof B: tool and continuation.** Declare one harmless synthetic function
   using the currently documented supported namespace/`additional_tools` form.
   Request a call, buffer it to completion, and validate its name/JSON arguments.
   Record separately the output-item ID and `call_id` (use synthetic aliases in
   committed evidence). Supply a deterministic synthetic result without executing
   model-selected commands. Open a **new** Responses request with full required
   history, the assistant `function_call` and `function_call_output` using the
   same `call_id`; set `store: false`, `stream: true`, omit `previous_response_id`.
   Require a completed answer that uses that result. Record exact non-secret
   declaration/projection shape and whether opaque reasoning items were needed.
   A model requiring unrepresentable state does not pass the replay gate.
5. **Refresh rotation.** Near token expiry per current docs, serialize a
   form-encoded refresh to the documented token endpoint using
   `grant_type=refresh_token`, issued client ID, saved refresh token and
   `resource=https://api.openai.com/v1`; omit `scope` to retain the grant.
   Honor any documented refresh timing constraint including `earliest_refresh_at`.
   Require success and a replacement refresh token; compare in memory without
   printing values/hashes. Atomically persist the new access token, replacement
   refresh token, scopes, expiry and local generation together. Restart the
   harness, load the replacement record and complete another inference. Never
   deliberately replay an old refresh token. `invalid_grant` requires new login,
   not a retry loop. Record timing and rotation/persistence booleans only.
6. **Cleanup.** Revoke the renewable session via discovery's revocation endpoint
   using the refresh token and issued client ID, then clear local tokens. Record
   only whether remote revocation was confirmed; network failure is not proof
   of revocation. Never commit the credential directory or raw capture files.

## Safe evidence and exit review

Create a separately reviewed redacted proof report only after execution. Record:
UTC execution date, harness revision, independent-registration boolean,
user-confirmed workspace type (no name/ID), selected model, sanitized catalog
schema, outbound non-secret body shapes, event names/order, completion flags,
call-ID correlation/replay result, reasoning-state requirements, refresh
rotation/atomic-save/restart outcomes, and cleanup status. Record failures
honestly with sanitized status/error codes. Review manually before committing.

Never retain tokens (including ID tokens), codes, PKCE verifiers, state/nonce,
raw authorization/callback URLs, account emails/subjects, host/client identifiers,
private prompts, raw HTTP headers/bodies or browser screenshots in repository
evidence. Diagnostic request IDs should stay in private support records.
Public documentation URLs and a bare callback template are not login artifacts.

Phase 0 exits only when text and tool replay pass on at least one intended
model, the target workspace catalog shape and independent refresh rotation are
captured, and the device decision is recorded. All live gates above are still
unchecked. API-key tests, fake responses, and documentation research do not
clear them. Project-license confirmation is a separate unresolved decision.
