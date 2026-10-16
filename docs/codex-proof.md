# Isolated native Codex device-login proof

This is the opt-in Phase 0 probe for [ADR 0004](decisions/0004-pi-style-codex-authentication.md),
not production `atuin-ai-server` authentication. No Pi/Codex process is started
and no existing Pi/Codex credentials are accessed. The independent SIWC probe
is separate and deferred.

```sh
go build -trimpath -o bin/codex-proof ./cmd/codex-proof
./bin/codex-proof login --approve-live --device-page
```

For this remote interactive test only, a private device-code display is served
at `http://127.0.0.1:1455/`. Reuse the existing loopback SSH local forward to
view it in the workstation browser. It displays an activation link and the
short-lived user code; open the link and enter the code privately. This is
**not an OAuth callback**: no authorization code is redirected through the
tunnel. Device auth itself needs no tunnel; a later production CLI can display
the code in a suitably checked private terminal. This controlled probe requires
`--device-page` and never prints the code to process output.

Login expires after 15 minutes. Do not record the terminal/browser, take code
screenshots, or paste codes, URLs containing secrets, tokens or account details
into chat. Use the intended Business workspace. A local process/user can reach
the loopback display; this assumes a trusted personal host, not multitenancy.

The isolated credential record is at `~/.atuin-ai-codex-proof/record.json`.
The directory is `0700`, files are `0600`, operations hold an exclusive lock,
and saves use fsync/atomic rename. Keep this path outside the checkout. Login
refuses to overwrite a retained session, including one with expired access
credentials. Do not bypass this by copying or deleting another app's files.

After login, safe status and opt-in synthetic inference commands are:

```sh
./bin/codex-proof status --approve-live
./bin/codex-proof prove --approve-live --model gpt-6.1-sol
```

The model is a source-catalog candidate, not a claim of account entitlement.
The proof performs exactly three generations: a greeting, a harmless no-argument
function, and a subsequent full-history `function_call_output` continuation.
It never executes a command or accesses files on behalf of the model. The
report has booleans/model/generation only. SSE parsing is bounded and requires
explicit completed snapshots matching streamed text. Opaque reasoning items
are deliberately omitted in this replay experiment and their presence is
reported; failure means no stateless compatibility proof. This is not a general
production SSE implementation or permission to silently discard required state.

Refresh and local cleanup are explicitly opt-in:

```sh
./bin/codex-proof refresh --approve-live
./bin/codex-proof prove --approve-live --model gpt-6.1-sol
./bin/codex-proof logout --approve-live
```

Refresh uses the same isolated account/session, requires replacement refresh
tokens and atomically persists them before further use. It does not retry a
failed exchange. Run refresh near expiry; do not probe old refresh tokens or
loop requests. Interrupted saves can require reauthentication; crash-orphaned
private temporary records are removed under the store lock. Local logout clears
this probe's local credential record only. **Remote revocation is not confirmed
or implemented**; manage the connected Codex session through OpenAI as needed.

Access-token account metadata is decoded from the token returned by the fixed
HTTPS token endpoint. This is **not JWT signature/OIDC verification**, and is
not the independent SIWC identity-verification contract. No API-key billing
fallback occurs automatically. HTTP redirects and automatic retries are disabled.

Actual outcomes are recorded separately in the
[redacted Phase 0 report](proofs/codex-phase0.md): login and text/tool replay
and refresh/restart passed; cleanup remains an explicit follow-up. This instruction
document is not itself live validation evidence.
