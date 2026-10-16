# Deferred opt-in SIWC proof harness

> Independent SIWC is deferred by [ADR 0004](decisions/0004-pi-style-codex-authentication.md).
> This harness does not implement the selected Codex device flow. A live attempt
> reached agent naming and plan-usage consent, then organization policy denied
> completion with `3p_delegated_access_policy_denied`. No token exchange or
> inference success was demonstrated. Retain this probe for future SIWC work.

This harness is an independent integration probe, not the production OAuth
credential manager or Atuin chat implementation. It does not use Codex/Pi
registrations, credentials, private endpoints or device authorization.

## Procedure

1. Confirm workspace policy permits the test and approve a small amount of
   ChatGPT-plan usage. Keep the SSH callback tunnel in
   [deferred SIWC instructions](authentication-siwc-deferred.md) open.
2. Build the harness and run its login command with explicit live approval.
   Open `http://127.0.0.1:1455/` on the browser workstation. Click **Continue
   with ChatGPT** and select the intended Business workspace. The authorization
   URL stays in the browser flow, not command output or repository evidence.
3. Run the catalog/inference probe with explicit approval. It selects a
   preferred visible model from the real catalog, or requires an explicit model
   if none matches. Three Responses generations test streamed greeting, a
   namespaced synthetic function, and a full-history continuation. The function
   has no arguments and returns a constant marker; no commands or file access
   are possible. The continuation must use that marker. Call IDs are used for
   correlation and output-item IDs are not substituted.
4. Refresh only when the returned refresh timing permits it. Replacement tokens
   must be saved together. Restart the probe and repeat inference using the
   persisted record. This can require waiting; never force or repeatedly retry
   an early refresh.
5. Revoke the renewable session and clear local tokens. A failed revocation must
   be reported, not called successful cleanup; disconnect the app in ChatGPT
   Settings if necessary.

Exact command usage is exposed by `cmd/siwc-proof`. There is no SIWC auth command
in `atuin-ai-server` yet. Keep the proof isolated from the phase 1 HTTP shell.

## Safety and evidence

Do not run login under HTTP tracing, shell tracing or terminal recording. Never
paste callback URLs, authorization URLs, token records or screenshots here.
The browser landing/callback pages must send no-store and no-referrer headers;
no external resources are loaded. Credential storage is owner-only, outside the
checkout, with a single writer. No live calls run in ordinary tests or CI.

The probe bounds JSON responses, SSE events, total SSE bytes and output text,
rejects failed/incomplete streams and premature EOF, and disables HTTP redirects
and automatic retries. A successful terminal snapshot is required before a
synthetic function result is submitted. This probe is deliberately narrower
than the future production parser: it does not claim phase 5 completion.

Opaque reasoning items are deliberately omitted in the replay experiment and
the report records their presence. Success proves only this specific model and
transcript replay; it is not permission to silently discard required provider
state in production. Failure does not validate stateless compatibility.

Only sanitized booleans, catalog shape/count and selected model belong in a
reviewed proof report. No successful results have been recorded by this
instruction document.

## Official sources checked before the probe

- https://developers.openai.com/siwc/token-sharing-open-source/sign-in
- https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference
- https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions
- https://developers.openai.com/siwc/token-sharing-open-source/token-reference
- https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations
- https://developers.openai.com/api/docs/guides/function-calling (namespace schema)
- https://auth.openai.com/.well-known/openid-configuration
