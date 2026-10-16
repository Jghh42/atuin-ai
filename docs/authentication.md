# Authentication: initial Codex path and deferred SIWC

## Current decision

[ADR 0004](decisions/0004-pi-style-codex-authentication.md) selects native
Pi-style Codex device login and direct Codex-backend inference for the initial
implementation. Independent SIWC browser login and public Responses inference
remain a future feature, not an initial release gate. This replaces the former
public-Responses-only rule; it is not merely a new login UI for the same token.

**Implementation status:** Phase 7 implements production device login, storage,
rotating refresh and logout, composing the Phase 6 SSE pipeline with one shared
credential manager/catalog. Initial reviews required shutdown-join and Unicode-
fold fixes plus formatter hardening. Both independent re-reviews cleared those
fixes with no new blockers; independent post-fix verification also passed. Fresh
production login/live lifecycle is **NOT PASSED**, and the live eligible catalog
remains **NOT PASSED**. The isolated [`cmd/codex-proof`](codex-proof.md) outcomes,
including a [fresh retained-session recheck](proofs/codex-retained-probe-2026-10-16.md),
are PROBE evidence only; no proof-store or third-party credentials are imported. The SIWC harness is a separate deferred experiment.

## Initial login design

`atuin-ai-server auth login` explicitly requests a device code with the reviewed
fixed registration `app_EMoamEEZ73f0CkXaXp7hrann`. It displays only the fixed
`https://auth.openai.com/codex/device` URL and the user code on the controlling
terminal (`/dev/tty`), not stdout/stderr logs. Noninteractive login is refused.
Approval may occur in a browser on another machine; no callback listener/tunnel
is used. `auth status` reports local state only, without refreshing; `auth logout`
deletes local secrets durably. Stop `serve` before any separate auth command:
they all share one nonblocking lifetime Unix file lock.

State is NEW application-owned `os.UserConfigDir()/atuin-ai/credentials` (Linux:
`$XDG_CONFIG_HOME/atuin-ai/credentials`, or `~/.config/atuin-ai/credentials`;
macOS: `~/Library/Application Support/atuin-ai/credentials`). Configuration
remains optional, with only port/default_model. There is no token-import or
upstream URL setting. Never read/import existing Pi/Codex/proof auth files,
browser cookies, or their rotating session. No pending codes, authorization
codes/verifiers or raw token responses are logged or persisted.

The application directory is euid-owned 0700, regular records/lock 0600 with one
hard link. Every ancestor is walked with descriptor-relative nofollow opens;
foreign ownership, writable non-sticky ancestry and symlinks are rejected. Only
root-owned sticky temporary ancestry is permitted for isolated roots. All later
operations use the anchored directory descriptor. Same-UID processes/root are
trusted; this is not protection against a compromised account. Linux and macOS
are targeted; no Windows or network-filesystem/multireplica guarantee is made.
Plaintext private files are not encryption, secure erasure or an OS keyring.

### Rotation and restart safety

`Current` returns an atomic provider/account/auth-epoch/access-token pair.
Refresh starts at access expiry minus 60 seconds, is serialized/coalesced, and
has a 30-second worker budget with 20-second HTTP requests. Native transport is
HTTP/1-only, does not pool connections, and allows at most six per host; this
avoids standard-library raw unsolicited-idle-response/HTTP2 diagnostic logs.
Redirects are refused. Individual waiter
cancellation does not cancel other waiters or an in-progress rotation. Manager
shutdown cancels HTTP and joins workers while retaining the store lock. Every
concurrent Close caller joins the SAME complete shutdown, including detached
persistence and store/flock release, and receives the same store-close result;
source unavailability during closing does not mean shutdown is already complete.

Before ANY external refresh, a secret-free `intent.json` is atomically written,
file-synced, renamed and directory-synced. It overrides the old token record on
restart. There is exactly one refresh attempt: timeout/cancellation, ambiguous
transport failures, invalid_grant, account change, missing/unchanged replacement
refresh token or post-rotation persistence failure require explicit reauthentication.
If intent persistence fails BEFORE HTTP, no refresh was sent; restart may safely
retain the prior session in that case. Old refresh tokens are NEVER retried
after an ambiguous rotation. The intent also
covers explicit login and logout. Starting fresh login makes the previous local
session unavailable even if approval is subsequently cancelled.

A replacement record is completely written to an exclusive same-directory 0600
temporary file, synced, renamed and directory-synced before the intent is
removed and its removal directory-synced. It is published only after completion.
If intent cleanup fails, the current manager remains unavailable; after restart
either the intent blocks use or the ALREADY durable replacement is available,
never the old refresh token. Once tokens arrive, persistence has a separate
five-second budget independent of caller cancellation/shutdown. The budget is
checked between filesystem syscalls; an OS-stalled synchronous fsync cannot be
forcibly timed out safely. No background unjoined save or fallback use is allowed.

Records validate bounded strict JSON, schema/provider/registration, token and
account shape/binding, dates and counters. Access claim decoding is **NOT OIDC
signature verification**; tokens come only from the fixed HTTPS token endpoint.
Expiry uses the earlier of expires_in and the access-token exp claim. Refresh
requires the same account and a new refresh token. Generation increases on every
lifecycle transaction, including intents/tombstones; fresh login increases a
separate auth_epoch even for the same account. Rotation preserves auth_epoch.
JSON duplicate detection uses Unicode SimpleFold equivalence matching Go's
struct-field aliases, including long s (ſ) and Kelvin sign (K); lowercasing alone
is insufficient. Persisted field names remain exact, and numbers remain lossless.
Secret structs implement fmt.Formatter plus String/GoString and slog.LogValuer:
unsupported formatting verbs emit only a fixed redaction label. Intentional
private-store JSON serialization is unchanged.

Counter overflow fails rather than wrapping; failed in-memory reservations may
leave counter gaps, not backward/reused values. Private orphan temporary records
are safely removed while holding the lifetime lock; unsafe entries fail closed.

Logout replaces the secret-bearing record with a secret-free tombstone retaining
counters. This is durable local deletion, not forensic media erasure. No verified
remote revocation endpoint exists here; none is guessed or invoked. Pending
intents require explicit new login or logout, never manual replay of old tokens.
A corrupt/unsafe store is unavailable and requires operator investigation; do
not edit records, copy credentials or delete/replace the lock inode while any
process is running.

### Catalog / inference binding

The source publishes unavailable/new state, then synchronously invalidates its
single bound catalog outside state locks before finishing lifecycle operations.
Successful ordinary rotation preserves catalog identity; refresh failures clear
it. In-flight old catalog results cannot repopulate after logout/reauth. The
catalog identity is also pinned through the later inference credential lookup:
a request resolved for one account/epoch cannot silently use another's token.
Successful saved login is NOT rolled back if discovery fails; the command reports
`Login saved successfully; models unavailable`. No eligible models are invented.

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
probe, not by existing Pi usage. Phase 7 production lifecycle/security hardening
is now implemented and independently reviewed, not live-validated. Cleanup
remains an explicit follow-up, not evidence of remote revocation. API-key inference is a separately billed fallback.

## Production live validation — opt-in, NOT PASSED

No production login or live calls were attempted during Phase 7 implementation
while the user was AFK. Phase 0 proof is NOT a production pass. The historical
eligible-catalog probe returned no replay-proven model; one later candidate
probe failed before text completion, and no retries/alternate probes are
permitted by this checkpoint. Do not expand eligibility or fabricate a catalog.

The user is now available and has authorized their own login, its automatic
discovery and the initial serve/models workflow. Both security-fix re-reviews
and independent post-fix verification are cleared; the coordinator will invite
the user to run the flow in their own terminal. No outcome is inferred here.
[Exact bounded attended login/catalog commands](plans/phase7-attended-login-handoff.md)
are instructions, not executed-test evidence. The user must type login in their
OWN controlling terminal without capture or sharing the device code. The handoff
keeps unavailable discovery blocked and does not start Phase 8 real-CLI testing.

With security-fix re-review cleared, the existing user authorization covers
login and ordinary bounded metadata reads inherent in its initial serve/models
workflow without repeated approvals. Later inference/probe work remains bounded
separately. The broader lifecycle checklist is:

1. Build (`make build`), stop any server, and use an isolated, private application
   config directory (e.g. an operator-created 0700 `XDG_CONFIG_HOME` on Linux;
   never reuse Pi/Codex/proof directories). Run `auth status`, then `auth login`
   in an interactive terminal. Approve the fixed activation URL/code privately.
   Never capture the terminal, code, raw responses or state files in evidence.
2. Confirm the command distinguishes saved login from unavailable models.
   Unavailable discovery remains a BLOCKED eligible-catalog gate, not a failed
   save or permission to retry login, probe candidates or expand eligibility.
3. Run `serve` without model configuration and make the authorized initial local
   models GET, including if the login-time metadata read was unavailable. This
   ordinary server-time catalog fetch is included in the existing authorization;
   record both outcomes separately, without polling. If no eligible model is
   available, the gate remains BLOCKED. Only with eligible discovery and separately
   bounded inference approval, verify completed production streamed text, a
   harmless synthetic tool call and full-history tool-result continuation.
   Do not start Phase 8 real-CLI/differential testing from this checklist. Retain
   only sanitized outcomes and model ID.
4. Stop the server before `auth status`; record only generation/auth_epoch and
   expiry. Restart and keep the session until expiry-minus-60s, then issue one
   approved small inference to trigger rotation. Stop again and check generation
   increased while auth_epoch stayed unchanged. Do not force refresh by editing
   files or copying old tokens. Restart once more and verify one inference.
5. Stop, run `auth logout`, then `auth status`. Restart: models/chat must be 503.
   Record local deletion only, not remote revocation. Do not inspect private
   file contents to demonstrate deletion. Clean any isolated test environment
   only after all processes stop; preserve the application lock inode during use.

This is a manual opt-in production validation path, not an automated live test
or a claim that Phase 8 real-CLI compatibility/resource gates have passed.
Deterministic fake transport tests cover the composed lifecycle separately.

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
