# Deferred SIWC research and future live proof

> **Deferred by [ADR 0004](decisions/0004-pi-style-codex-authentication.md).**
> The initial path is native Pi-style Codex device login. This checklist is
> retained for a later independent SIWC integration, not an initial release gate.

## Status

**SIWC validation: incomplete and deferred.** A live browser attempt reached
agent naming and consent for profile information and ChatGPT-plan usage, then
failed with `3p_delegated_access_policy_denied` and an organization-access
message. No successful OAuth exchange, model listing, inference, tool
continuation or refresh was demonstrated. No intended model is validated for
SIWC. Existing Codex/Pi subscription login is not independent-client evidence.
Public official
documentation was retrieved over HTTPS on 2026-10-16; see
[ADR 0001](decisions/0001-upstream-authentication.md) for references and the
browser/loopback decision, and [ADR 0002](decisions/0002-stateless-responses-continuation.md)
for replay requirements.

The phase 1 HTTP shell and Go tooling are present, but do not implement auth
or inference. A separate `cmd/siwc-proof` probe exists; see
[its notes](siwc-proof.md). It is not the production credential manager.

## Prerequisites (all must be satisfied before the opt-in proof)

- [ ] User explicitly approves login, requested permissions, use of the target
  Business workspace/seat, and any plan usage. Confirm workspace policy permits
  this integration. Never infer eligibility from another application's login.
- [ ] A system browser is available on the user's local workstation. The
  callback listener runs on the service machine's `127.0.0.1`; for an SSH
  development host, establish the documented local port forward before login.
  Outbound HTTPS to `auth.openai.com` and `api.openai.com` must work.
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

## Remote development callback

For a remote development host, choose a fixed callback port and establish the
forward from the workstation before starting login:

```sh
ssh -o ExitOnForwardFailure=yes \
  -L 127.0.0.1:1455:127.0.0.1:1455 user@development-host
```

On the remote host, start login with that same port and browser launching
disabled. The command must print, but never log, the authorization URL. Open it
in the workstation browser. Its redirect to `127.0.0.1:1455` crosses the SSH
tunnel to the remote loopback listener. Keep the SSH session open until the
code exchange finishes. Do not bind the callback to `0.0.0.0` and do not expose
it through a public ingress.

Exact command flags will be finalized with the future SIWC CLI; the intended UX is:

```sh
atuin-ai-server auth login --callback-port 1455 --no-open-browser
```

Native device login is now the initial Codex path, not an extension of this
independent SIWC flow. SIWC credential-file import remains deferred.

## Opt-in execution checklist

1. **Register/sign in.** Start `http://127.0.0.1:<selected-port>/auth/callback`
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

This future SIWC integration is validated only when text/tool replay passes
on an intended model, catalog shape and independent refresh rotation are
captured, and direct/SSH-forwarded loopback login is verified as applicable.
These SIWC success gates remain unchecked; the browser consent attempt did not
complete authorization. Codex/API-key tests, fake responses and documentation
research do not clear them. The active initial gates are in
[authentication.md](authentication.md). The separate Apache-2.0 project-license decision is confirmed in
[ADR 0003](decisions/0003-project-license-proposal.md).
