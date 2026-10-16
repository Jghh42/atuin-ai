# Phase 7 — production native Codex credential lifecycle

2026-10-16 independently reviewed code checkpoint, **cleared for authorized
direct-main delivery**. Both security re-reviews cleared the shutdown/Unicode/
formatter fixes with no new blockers; independent post-fix verification passed.
The Go toolchain/devtool/CI follow-up and test-only native-cancellation harness
fix are also independently reviewed and CLEARED; final exact-Go-1.27.2 full
checks/build/diff and all-six-package race x20 passed.
Fresh production live lifecycle and eligible-catalog gates remain **NOT PASSED**.
The delivery report/next ledger records the exact commit and CI result, not an
own-hash assertion inside this checkpoint.
Started from clean main `14648ff9ab610dc4a5bae915f5b2a2e469ba2e18`; prior exact-HEAD
Go checks run `38959509541` was green (coordinator-verified handoff).

## Implemented design

- `internal/credentials`: native fixed-registration device usercode/poll/token
  flow and returned authorization-code + PKCE-verifier exchange, using pinned
  Pi AI 1.1.0 protocol references, not its runtime/store. Fixed activation/device
  redirect and HTTPS token endpoint only; no browser/SIWC/DCR/scopes import.
  15-minute login, 20-second HTTP calls, explicit body/header/dial/TLS limits,
  no redirects, bounded intervals, pending/slow_down/expiry/denial handling.
  Native transport is HTTP/1-only without pooling (six connections/host maximum),
  avoiding net/http's raw unsolicited-idle-response and separate HTTP/2 logs.
  Explicit error codes win over pending HTTP statuses and success. Malformed
  nonempty pending bodies fail closed. Strict verifier, token, account/header,
  expiry and duplicate/Unicode/depth checks. JWT account/exp DECODE is **NOT OIDC
  signature verification**. The earlier claim/response expiry is used.
- `Manager.Current` implements the existing atomic identity/access-token source.
  One serialized/coalesced refresh worker; independent waiter cancellation,
  60-second expiry skew, fixed clock/transport/store-fault test seams. Fresh login
  advances auth_epoch even for the same account; rotation preserves it. A
  separate persisted generation advances on lifecycle transactions; failed
  reservations may leave gaps, never wrap or reuse a lower counter.
- A durable secret-free `intent.json` precedes ANY external refresh side effect.
  It overrides the old record after restart. Ambiguous HTTP/timeout/cancellation,
  invalid_grant, missing/unchanged replacement refresh token, account change or
  post-rotation save failure blocks further use and requires explicit login.
  There is no old-token retry. If Begin fails before durable intent, no external
  refresh is sent: restart may safely retain the prior session in that case.
- After token arrival, a separate five-second persistence budget is independent
  of caller cancellation/shutdown. No replacement is published before complete
  write/file-sync/rename/directory-sync and intent cleanup. Failed cleanup can
  yield an unusable intent OR the ALREADY durable replacement after restart,
  never the old refresh token. OS-stalled synchronous filesystem calls cannot
  be forcibly interrupted; budget checks occur between syscalls. No detached
  unjoined persistence goroutine is used.
- The euid-owned 0700 application directory is walked/anchored through Unix
  descriptor-relative nofollow opens. Later opens/rename/unlink stay relative
  to that capability. Records and permanent lock are regular 0600, one-link,
  euid-owned; unsafe writable/foreign ancestry and symlinks are rejected (only
  root-owned sticky temporary ancestry is permitted). Linux/macOS implementation;
  no Windows/network-filesystem/multireplica guarantee. Root/same-UID compromise
  is outside the trust boundary. Plaintext private files are not encryption.
- Bounded 192 KiB strict records validate exact schema field names, provider,
  registration, dates, counters, tokens and claim/account binding. Atomic saves
  use exclusive same-directory private temporaries, complete writes, file sync,
  rename and directory sync. Safe orphan cleanup runs under the lifetime lock;
  unsafe entries fail closed. Lock inode is never unlinked or replaced.
- Explicit logout durably replaces the secret-bearing record with a secret-free
  counter tombstone. Local deletion is not forensic erasure or remote revocation.
  No supported revocation endpoint was verified, so none is guessed/invoked.
- `internal/application` composes ONE lifetime manager, ONE account/epoch-bound
  cache shared by listing/chat and native `RequestClient.Stream`. Config remains
  optional port/default_model, default 17391; no model list/upstream URL settings.
  State is NEW `os.UserConfigDir()/atuin-ai/credentials`; no Pi/Codex/proof import.
- Lifecycle changes publish unavailable/new source state then synchronously
  invalidate outside state locks. Successful rotation retains catalog identity;
  failed rotation invalidates before waiters finish. Same-account reauth/logout
  cannot repopulate from old in-flight discovery. Snapshot identity is pinned
  through the inference source lookup; changed account OR epoch rejects instead
  of rebinding a request to a new session.
- CLI: `auth login`, `auth status`, `auth logout`, `serve [--config PATH]`. All
  share the same nonblocking lifetime Unix lock: stop server before separate auth
  commands. Login requires a controlling terminal and sends the fixed activation
  URL/user code ONLY there, never logs/persists pending artifacts. Status reports
  local state and safe counters/expiry, not account IDs or upstream acceptance.
  Saved login remains successful if discovery fails; output clearly says models
  unavailable. Missing credentials/eligible discovery yields safe models/chat
  503; eligible authenticated fake discovery drives the real Phase 6 SSE pipeline.
- Secret structures implement fmt.Formatter, String/GoString AND slog.LogValuer
  (JSON logging does not inherently honor Stringer). Unsupported verbs emit
  only a constant redaction label; intentional store JSON is unchanged. CLI/API
  errors are generic and never include raw bodies, tokens, account IDs or
  filesystem paths.

## Deterministic validation and scope

Tests use only synthetic credentials, isolated private directories and injected
HTTP transports. No browser, third-party credentials, live login, discovery or
model probes were used.

- Device pending and 403/404 fallback, nested/string slow_down, error precedence,
  denial/expiry, cancellation at acquire/wait, malformed/oversized responses,
  unsafe display code, strict PKCE exchange, fixed endpoint/registration forms,
  no redirects and finite request deadlines.
- Account binding, missing/unchanged refresh replacement, invalid_grant, claim
  expiry, strict numeric expiry and earlier expiry selection; exact skew boundary.
- Concurrent rotating Current calls (32 waiters), canceled waiter isolation,
  shutdown cancellation/joining; ordinary rotation epoch/cache preservation.
- Durable intent visible before external refresh; transport/malformed/rotation
  failures are unusable on restart, no retry. Write/fsync/rename/dirsync faults
  at BOTH intent/replacement stages; no HTTP side effects before successful
  intent. Intent cleanup faults cannot restore old tokens. Explicit login can
  recover a failed Begin counter reservation. Login and refresh persistence
  survive caller cancellation; failed login/logout saves never claim success.
- Permissions, ancestry, symlinks, hard links, FIFO/nonregular files, corruption,
  oversized/unknown/duplicate/case-aliased fields, invalid Unicode/depth, orphan
  cleanup, descriptor-root path replacement and lifetime lock contention in
  same/separate processes. Ownership mismatch test is present but **skipped on
  this non-root runner**; euid ownership checks themselves are implemented.
- Same-account reauth advances epoch; logout tombstones retain counters and
  remove logical secrets. In-flight catalog isolation and inference identity
  mismatch regressions cover both changed account and same-account new epoch.
- CLI saved-login/model-unavailable distinction, status/logout/restart locking,
  argument/error/status and fmt/JSON-slog redaction.
- Composed real-loopback fake lifecycle: no-login 503 → explicit device login →
  eligible discovery → streamed text → buffered local tool call → full-history
  tool result continuation → rotating refresh → restart → native inference →
  local logout → models/chat 503. Exact upstream generation/refresh counts and
  access-token/account header pairs are asserted; no additional model is enabled.

Validation results are recorded after final checks below. Initial full check
caught only three staticcheck ST1005 error-string capitalization violations;
these were corrected without changing policy. No fuzz failure was suppressed.

One later race x20 sweep failed the pre-existing native provider test
`TestBlockedReadClosedOnTimeoutAndCancel` with `reader left blocked` (original
output `/tmp/phase7-final-race20-failed.log`). That original test had no
opened/read-state diagnostics, so the precise observed runtime cause is NOT
proven. Its 1 ms sleep could cancel before the transport returned a body, yet
it unconditionally demanded that the preconstructed body be closed. A new
deterministic pre-open cancellation regression proves that ownership assumption
is unsound; it does not retrospectively prove the exact failing interleaving.
A diagnostic-only stress rerun x300 passed and likewise does not diagnose it.
The blocked-reader test now waits for actual Read entry, then injects cancellation
or a deadline cause and asserts prompt close/join with finite guards. Existing
real-TCP header/body inactivity and generation-timer tests remain unchanged.
No native stream runtime code was changed. Focused handshake/pre-open/TCP race
x50 passed. Final full checks/race results are recorded below.

## Initial implementation validation results

These are the pre-security-fix writer results, not a claim of independent
clearance for the follow-up changes below.

- Final `make check`: **PASS** (unit/race, vet, pinned staticcheck, govulncheck
  with no vulnerabilities, all four bounded fuzz smoke targets).
  Log: `/tmp/phase7-final-check.log`.
- Final `make build`, macOS arm64/amd64 cross-build and `git diff --check`:
  **PASS**. macOS execution is NOT tested.
- Final targeted credential/application/catalog/httpserver/native-provider/CLI
  race **x20 PASS**. Log: `/tmp/phase7-final-race20.log`.
- Focused blocked-read handshake/pre-open/native TCP race **x50 PASS**.
  Log: `/tmp/phase7-stream-handshake-race50.log`.
- These final passes follow the recorded test-only ownership-handshake change;
  they do not prove the exact cause of the earlier failing interleaving or the
  historical FuzzDecode anomaly. The subsequent fix re-review is recorded below.

## Independent-security-review fixes — re-review cleared

The two read-only reviews found no criticals but identified two must-fix
warnings. Independent verification of the ORIGINAL implementation's full checks,
race x20 and Darwin cross-builds passed. Subsequent independent post-fix checks
and both focused security re-reviews also passed, as distinguished below.

1. **Concurrent Close join:** closing is now distinct from fully closed. The first
   caller cancels/invalidates, joins workers and detached persistence, serializes
   store release, then publishes one shared done/result. Every concurrent AND
   later Close caller joins that completion and returns the same store-close
   outcome. New operations reject both closing and closed states. The regression
   holds persistence and final store Close at separate handshakes, checks two
   Close callers cannot finish or permit flock reacquisition early, then proves
   durable replacement, one storage Close, joined waiters and identical nil/error
   results for concurrent and later callers.
2. **Unicode-fold JSON aliases:** duplicate canonicalization now uses the least
   rune in each Unicode SimpleFold orbit, matching Go EqualFold/encoding-json
   aliases, not strings.ToLower. Long s (ſ), Kelvin sign (K), Greek sigma and
   ASCII aliases are rejected at root and nested boundaries. Additive distinct
   names remain accepted, numbers preserve exact lexemes, and persisted field
   names remain exact. The reported access-token overwrite pair is a regression.
3. **Formatter redaction:** all secret-bearing types now implement fmt.Formatter
   emitting only a constant label for every verb/flag/width/precision. Tests cover
   value/pointer forms with %d/%x/%q/%s/%v/%+v/%#v and formatting mistakes, JSON
   slog and unchanged intentional access/refresh/account store serialization.

`manager_close_test.go` adds finite gated shutdown regressions; `json_test.go`
and `manager_test.go` extend alias/numeric/formatting coverage. No live attempt
or retained-probe credential access was used.

Post-fix validation: **PASS** for `make check` (unit/race, vet, staticcheck,
govulncheck with no vulnerabilities, four bounded fuzz targets), `make build`,
`git diff --check`, Darwin arm64/amd64 cross-builds and all six runtime/CLI race
packages x20. Focused Close/Unicode/numeric/formatter regressions also passed
race x20. Logs: `/tmp/phase7-security-fixes-check.log` and
`/tmp/phase7-security-fixes-race20.log`. macOS execution and privileged ownership
mutation are NOT claimed. Both independent security re-reviews then cleared the
fixes with no new blockers. Independent post-fix `make check`, build, diff check
and credential/application/catalog race x20 also passed. The independently
reviewed code checkpoint is cleared for delivery, not a production-live pass.

The attended handoff now reflects the user's existing login/automatic-discovery/
initial serve/models authorization: ordinary bounded metadata reads need no
redundant new approval. Unavailable catalogs remain blocked; no blind retries,
candidate inference or eligibility expansion is authorized. Review clearance is
now satisfied; the coordinator will invite the user to perform fresh login in
their OWN controlling terminal. No live outcome is inferred by this checkpoint.

## Go fuzz toolchain and native-cancellation follow-ups — CLEARED

A fresh delivery preflight under Go 1.26.9 failed FuzzDecode with only a five-second
budget-boundary `context deadline exceeded`. The log is preserved at
`/tmp/phase7-delivery-check.log`; delivery paused without commit/push or blind
reruns. Subsequent read-only diagnosis established the exact known vulnerable
Go coordinator branch and a finite native-context reproduction (11/20000), not
an instrumented trace of either historical failed invocation.

Official metadata/release history verified **Go 1.27.2** as the current stable
security-patched 1.27.x release; its actual downloaded, unmodified source has the
upstream parent-plus-child guard at internal/fuzz/fuzz.go:129–137. `go.mod` is now
the exact Make/CI pin and minimum; workflows verify and export that toolchain.
Fuzz targets, seeds, 5s smoke/2m nightly budgets, assertions and failure behavior
are unchanged. No SDK dependency upgrade or production auth logic change.
[Source references, precise confidence and new-compiler verification](fuzz-toolchain-diagnosis.md).
Verified compatible DEVELOPMENT-tool pins are Staticcheck v0.8.2 and govulncheck
v1.9.0; x/tools v0.52.0 nilness explicitly covers nil-pointer analysis retired
upstream by Staticcheck. The old tools' export-data/SSA failures are preserved.
New-compiler full check/build (all four unchanged fuzzes), five independent
5s decoder campaigns, Darwin cross-builds and the requested five-package race
x20 checks PASS. An additional httpserver race check FAILED nonreading native
cancellation; that combined command remains a preserved failure. Separate
[controlled TCP diagnosis](../provenance/native-cancellation-diagnosis.md) then
proved the original test's implicit backpressure assumption unsound, without
claiming the exact historical interleaving. A test-only harness fix observes real
partial TCP timeouts, uses explicit handshakes/bounded buffers and retains
cancellation, admission, event-to-cleanup deadlines and no retries. Focused review
CLEARED it; race x50 observed 300 actual timeouts plus 300 disconnects.

Focused tooling review CLEARED the exact compiler/devtool/CI pins and nilness
substitution. FINAL independent verification PASSed exact Go 1.27.2 `make check`
(all analysis tools and FOUR unchanged 5s fuzz targets), `make build`, diff check
and credentials/application/catalog/httpserver/native-provider/CLI race x20.
No functional runtime/auth code changed in these follow-ups. [Detailed sources,
old failures and final disposition](fuzz-toolchain-diagnosis.md#reviewed-native-cancellation-follow-up-and-final-verification).
All review/verification blockers for this code checkpoint are cleared; exact
commit/CI result belongs in its delivery report, not inside its own commit.

## Unpassed / deliberately blocked gates

- **Fresh production device login/live lifecycle: NOT PASSED.** No production
  live/interactive attempt occurred during AFK implementation. The user is now
  available; both fix re-reviews and independent verification cleared the code.
  [Bounded attended login steps](phase7-attended-login-handoff.md) are prepared
  for the coordinator's user-owned-terminal invitation. Any later live outcomes
  are recorded separately, not inferred here. The separately authorized
  [fresh retained-PROBE recheck](../proofs/codex-retained-probe-2026-10-16.md)
  passed six generations and one refresh (generation 2 → 3), with no login,
  logout or discovery. Neither it nor historical Phase 0 clears production gates.
- **Live eligible catalog: NOT PASSED.** The catalog omitted replay-proven
  `gpt-6.1-sol`. One historical candidate probe failed before text completed;
  no retries, alternate models, new discovery/probes or eligibility expansion.
- Both security-fix re-reviews cleared this code checkpoint for authorized
  delivery. Exact commit identity and exact-HEAD CI outcome belong in the delivery
  report/next ledger; they are not inferred from prior-SHA checks.
- Phase 8 real-Atuin/differential tests and Phase 9 resource/release hardening
  remain unimplemented/unexecuted. No keyring/multireplica/Windows expansion.
- Historical FuzzDecode deadline failures remain recorded in [progress](autonomous-progress.md).
  A known Go coordinator race is now established by source and finite model;
  the exact historical individual interleavings remain unproven. The Go 1.27.2
  toolchain remedy and test-harness follow-up are now reviewed/validated as
  above; no old failure was erased or converted to success.

## Review handoff paths / APIs

- `internal/credentials/{source,record,json,redaction,oauth,manager,store_unix}.go`:
  `Identity`, `BindIdentity`/`MatchesIdentity`, `Record`, `OAuthClient`, `Storage`,
  `OpenStore`, `Begin`/`Commit`/`Load`, `NewManager`, `Current`/`Login`/`Logout`/`Close`.
- `internal/credentials/{test_helpers,oauth,store,manager,manager_close,durability,json}_test.go`.
- `internal/application/{runtime.go,runtime_test.go}`: `NewWithOptions`, `Runtime`.
- `internal/catalog/catalog.go`: snapshot identity stamping/accessor.
- `internal/provider/codex/{request.go,identity_test.go}`: identity-bound opening.
- `internal/provider/codex/stream_test.go`: `closeReader`,
  `TestBlockedReadClosedOnTimeoutAndCancel` read handshake and
  `TestCancellationBeforeOpenDoesNotOwnUnacquiredBody`; validation-only changes,
  no Phase 5/6 stream runtime modification.
- `internal/httpserver/{server.go,chat.go}`: bind catalog identity through SSE;
  runtime comments, no change to Phase 6 terminal/tool semantics.
- `internal/httpserver/{native_cancel_test.go,native_cancel_trace_test.go}`:
  test-only real-TCP cancellation/backpressure/admission witnesses, independently
  reviewed; diagnosis at `docs/provenance/native-cancellation-diagnosis.md`.
- `cmd/atuin-ai-server/{main.go,auth_test.go}`: auth CLI/production composition.
- `go.mod`, `go.sum`, `LICENSES/Go-x-sys-{BSD,PATENTS}.txt`, `NOTICE`: pinned
  x/sys v0.49.0 and retained unmodified notices.
- README, authentication/operations, ADR 4, plan/progress/status and provenance
  documentation reflect implementation versus review/live blockers separately.
