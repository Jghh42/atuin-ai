# Local development server (reviewed Phase 7 code checkpoint)

Requires Go **1.27.2** minimum. The exact `go.mod` patch is also the local
Makefile and CI toolchain pin: Make exports `GOTOOLCHAIN=go1.27.2`, and CI verifies
setup-go selected that exact release before pinning all subsequent commands.
Normal automatic toolchain download is allowed; an older launcher is sufficient.
This officially published security-patched release contains the upstream fuzz
coordinator race fix; see the [sourced diagnosis](plans/fuzz-toolchain-diagnosis.md).
The 5-second smoke and 2-minute nightly campaigns are unchanged, with no error
filter/retry wrapper. Build with `make build`, then run:

```sh
# Attended live login only after explicit opt-in; production live gate NOT PASSED:
./bin/atuin-ai-server auth login
./bin/atuin-ai-server auth status
./bin/atuin-ai-server serve # config is optional
curl http://127.0.0.1:17391/api/cli/models # 503 unless login AND eligible discovery succeed
# Stop serve before auth status/login/logout in a separate process:
./bin/atuin-ai-server auth logout
```

Configuration precedence: `serve --config PATH`, then `CHAT_CONFIG`, then
`$XDG_CONFIG_HOME/atuin-ai/config.toml` (the platform's `os.UserConfigDir` on
other systems). A missing default file uses built-in defaults; an explicitly
selected missing file is an error. Files are read once with a 64 KiB bound;
key validation and typed decoding use the same byte snapshot. Only `port` (default **17391**) and optional
`default_model` are accepted. Remove obsolete `[[models]]`/alias mappings: they
produce a migration error. Overrides must match an eligible discovered upstream
ID, not a private/public alias mapping.

The executable now composes native Codex discovery/inference with one lifetime
credential manager and one shared account/epoch-bound catalog. It never imports
isolated proof, Pi or Codex state. `auth login` displays the fixed activation URL
and user code only to `/dev/tty`; there is no unattended/browser-cookie/import
path. Login is durably saved before discovery, and successful login with failed
or ineligible discovery is reported as saved with models unavailable, not rolled
back. Missing credentials or an eligible catalog gives safe JSON 503. Production
live login/lifecycle and the historical eligible-catalog gate are **NOT PASSED**;
see [authentication/security and opt-in steps](authentication.md).

`GET /api/cli/models` returns the visible replay-proven catalog; chat resolves
unknown IDs to that same snapshot's default. `httpserver.NewWithRuntime` accepts
credentials, that catalog, and the native streaming provider. Catalog-only or
uncredentialed library construction remains safely unavailable, never a
fabricated provider. Invalid decoded requests return 400 (`invalid_request` or
`conversation_too_large`); oversized bodies return 413 before discovery. Other paths return JSON 404; unsupported methods
on known routes return 405. See [decoder policies](compatibility.md).

Discovery limits: 2 MiB body, 256 models, 128-byte IDs, 256-byte display names,
4096-byte descriptions, eight-second fetch timeout, 15-minute freshness. A
transient transport/429/5xx failure may serve the same account's last validated
snapshot for at most one hour; auth/metadata/override errors clear it. Refreshes
coalesce, individual waiters can cancel, and the last cancellation stops the
fetch. Logout/reauth/account change and failed rotating refresh publish unavailable/new
source state then synchronously invalidate the cache. Successful rotation
preserves auth_epoch; fresh login always advances it, even for the same account.
Inference is pinned to discovery's identity and rejects later account/epoch
changes rather than rebinding a decoded request. No
fabricated fallback exists. Only `gpt-6.1-sol` has replay proof; the live catalog
currently omits it, so the live eligible-catalog gate is **NOT PASSED**. See
[discovery evidence](proofs/codex-discovery-phase3a.md).

## Private state and single-process operation

Production records live under `os.UserConfigDir()/atuin-ai/credentials`, NOT the
proof store (Linux default `~/.config/atuin-ai/credentials`; macOS
`~/Library/Application Support/atuin-ai/credentials`). The final directory must
be euid-owned 0700, records/lock regular 0600 without hard links; symlinked or
unsafe writable ancestry is rejected. State uses descriptor-relative nofollow
operations, strict bounded records and atomic synced replacement. There is no
encryption/keyring claim; protect your account and filesystem/backups. A hostile
same-UID process/root, network filesystems, Windows and multiple replicas are
outside this boundary. Do not print or copy private state to diagnose errors.

`serve` and every separate `auth` command acquire the same nonblocking lifetime
file lock. STOP the server before separate login/status/logout; lock contention
fails rather than waiting or racing another refresh. Never unlink/replace the
lock file to bypass contention. Shutdown retains the lock through cancellation
and completion of rotating credential work. Concurrent Close callers all join
the complete shared shutdown/store release and receive the same close result;
becoming unavailable is not itself completion of shutdown.

`auth status` reports local login/unusable state and safe generation/auth_epoch/
expiry only; it does not prove upstream acceptance or refresh the session.
`auth logout` durably replaces secrets with a counter-only tombstone; it does
not revoke remotely or securely erase disk history. A durable secret-free intent
precedes external token rotation. Ambiguous refresh/timeout/invalid_grant or
post-rotation save failure blocks all further use across restart: explicitly
log in again. A failed intent save sends NO refresh, so restart may safely retain
the old session in that pre-side-effect case.
There is no automatic old-token retry. Once tokens arrive, a detached bounded
persistence operation finishes despite caller cancellation. Filesystem budgets
are checked between syscalls, not a guarantee against a kernel-stalled fsync.
Secret structs redact all fmt verbs, including formatting mistakes, and JSON
slog output without changing intentional private-store JSON. Duplicate-member
validation follows Go's Unicode SimpleFold aliases, not lowercasing alone;
persisted field names remain exact.
Unsafe/corrupt state needs operator investigation, not guessed endpoints or
manual token edits. [Full lifecycle invariants](authentication.md#rotation-and-restart-safety).

## HTTP boundary

The port is configurable; the host is not. The listener is always IPv4
`127.0.0.1`. Client authentication is absent, and Authorization headers are
ignored. Do not expose this port with a proxy, SSH tunnel, port forward or
container port publication. Loopback does not protect against other local
processes/users or browser-origin attacks; this is a personal local process,
not a multi-user security boundary.

At most 64 TCP connections are retained; further accepted connections are
closed immediately without creating HTTP connection goroutines. The finite OS
TCP backlog is separate. At most four chat requests are admitted, including
body reads, with no waiting queue. Excess requests return 503 with `Retry-After: 1`. Each admitted body is
limited to 4 MiB (including chunked bodies); larger bodies return 413. Headers
have a 16 KiB configured limit and a five-second read deadline; full request
reads have a 15-second deadline. Idle HTTP connections expire after 60 seconds.
The initial HTTP write deadline is 310 seconds. SSE overrides it per frame and
flush with a five-second deadline (`Runtime.WriteTimeout` is test-injectable).
Unsupported deadline controllers fail closed; failed writes/flushes cancel
upstream immediately without retries. The runner bounds generation to 300 seconds
separately from downstream cancellation, permitting a generic terminal error on
upstream timeout. The native provider also enforces 60-second inactivity, 1 MiB
SSE events, 256 KiB arguments and 32 calls per response. Each chat makes exactly
one upstream generation, never executes tools, and never automatically continues.
Calls remain buffered until Stream succeeds and the whole batch passes schema,
capability, mixed-terminal, ID-collision and danger checks. Local calls end with
waiting_for_tools then done; suggestions end with done without waiting. Failures
use one fixed generic error, never raw upstream/write details. Each frame is
single-line JSON and flushed; session header and done agree. Usage includes all
spec fields: unknown counts (including cache creation) and all costs are null;
known counts, including explicit zero/cache reads, are preserved. No credits or
prices are invented. See [Phase 6 policies/tests](plans/phase6-status.md).

SIGINT/SIGTERM closes admission, allows active handlers up to five seconds to
drain, then cancels handler contexts and forcibly closes connections. Startup
and failures use JSON logs on stderr. Request bodies, headers, configuration
contents and upstream credentials must never be logged. Native upstream transport
uses HTTP/1 without connection pooling (six connections/host maximum), avoiding
net/http raw unsolicited-idle-response/HTTP2 diagnostics; redirects are refused. Configuration failures
are intentionally generic to avoid parser diagnostics exposing secret values.

Run `make check` for tests, race detector, vet, pinned staticcheck and
vulnerability scanning plus five-second bounded fuzz smoke tests for the
decoder, tool-batch validator, safety backstop and Responses SSE parser. `make build` creates the
binary. Run longer fuzzing by selecting `FuzzDecode` in `./internal/atuin`,
`FuzzValidateBatch` in `./internal/tools`, or `FuzzEscalate` in
`./internal/safety`, for example:
`go test ./internal/tools -run='^$' -fuzz=FuzzValidateBatch -fuzztime=1m -parallel=2`.
The nightly workflow runs each target for two minutes. No live OAuth checks
are run in CI.
