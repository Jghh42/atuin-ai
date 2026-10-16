# Local development server (through Phase 3a)

Requires Go 1.26.9 or newer (security fixes; Go's automatic toolchain selection
can download it when the installed compiler is older). Build with `make build`, then run:

```sh
./bin/atuin-ai-server serve # config is optional
curl http://127.0.0.1:17391/api/cli/models # currently 503 without injected credentials
```

Configuration precedence: `serve --config PATH`, then `CHAT_CONFIG`, then
`$XDG_CONFIG_HOME/atuin-ai/config.toml` (the platform's `os.UserConfigDir` on
other systems). A missing default file uses built-in defaults; an explicitly
selected missing file is an error. Files are read once with a 64 KiB bound;
key validation and typed decoding use the same byte snapshot. Only `port` (default **17391**) and optional
`default_model` are accepted. Remove obsolete `[[models]]`/alias mappings: they
produce a migration error. Overrides must match an eligible discovered upstream
ID, not a private/public alias mapping.

Native Codex discovery is implemented behind injected credentials/HTTP. The
standalone executable does **not** load the isolated proof store or third-party
credentials: production login/refresh/logout wiring remains Phase 7. Until then
models/chat return safe JSON 503. With injected credentials, `GET /api/cli/models`
returns the visible replay-proven catalog; chat resolves unknown IDs to that same
snapshot's default, decodes the protocol, and returns HTTP 501 for valid requests,
**not** SSE. Invalid decoded requests return 400 (`invalid_request` or
`conversation_too_large`); oversized bodies return 413 before discovery. No
inference provider is called yet. Other paths return JSON 404; unsupported methods
on known routes return 405. See [decoder policies](compatibility.md).

Discovery limits: 2 MiB body, 256 models, 128-byte IDs, 256-byte display names,
4096-byte descriptions, eight-second fetch timeout, 15-minute freshness. A
transient transport/429/5xx failure may serve the same account's last validated
snapshot for at most one hour; auth/metadata/override errors clear it. Refreshes
coalesce, individual waiters can cancel, and the last cancellation stops the
fetch. Logout/reauth/account change must synchronously invalidate the cache;
Phase 7 must implement the documented credential source/epoch contract. No
fabricated fallback exists. Only `gpt-6.1-sol` has replay proof; the live catalog
currently omits it, so the live eligible-catalog gate is **NOT PASSED**. See
[discovery evidence](proofs/codex-discovery-phase3a.md).

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
The HTTP write deadline is 310 seconds, and admitted chat processing has a
300-second context deadline after the bounded body read. Future provider work
must honor cancellation and enforce the separate 60-second inactivity timeout,
1 MiB SSE events, 256 KiB tool arguments, and 32 calls per response while reading
upstream. The Phase 3 batch validator enforces argument/call-count limits on
completed calls before any future emission; this does not replace bounded
provider parsing. Provider timeouts/event bounds remain constants until that
layer exists.

SIGINT/SIGTERM closes admission, allows active handlers up to five seconds to
drain, then cancels handler contexts and forcibly closes connections. Startup
and failures use JSON logs on stderr. Request bodies, headers, configuration
contents and upstream credentials must never be logged. Configuration failures
are intentionally generic to avoid parser diagnostics exposing secret values.

Run `make check` for tests, race detector, vet, pinned staticcheck and
vulnerability scanning plus five-second bounded fuzz smoke tests for the
decoder, tool-batch validator and safety backstop. `make build` creates the
binary. Run longer fuzzing by selecting `FuzzDecode` in `./internal/atuin`,
`FuzzValidateBatch` in `./internal/tools`, or `FuzzEscalate` in
`./internal/safety`, for example:
`go test ./internal/tools -run='^$' -fuzz=FuzzValidateBatch -fuzztime=1m -parallel=2`.
The nightly workflow runs each target for two minutes. No live OAuth checks
are run in CI.
