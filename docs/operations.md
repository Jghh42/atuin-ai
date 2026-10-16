# Local development server (phase 1)

Requires Go 1.26.9 or newer (security fixes; Go's automatic toolchain selection
can download it when the installed compiler is older). Build with `make build`, then run:

```sh
./bin/atuin-ai-server serve --config config.example.toml
curl http://127.0.0.1:8080/api/cli/models
```

Configuration precedence: `serve --config PATH`, then `CHAT_CONFIG`, then
`$XDG_CONFIG_HOME/atuin-ai/config.toml` (the platform's `os.UserConfigDir` on
other systems). A missing default file uses built-in defaults; an explicitly
selected missing file is an error. Unknown keys are rejected. No upstream
credentials are needed for this shell. Example model IDs are placeholders,
not a claim of SIWC availability or stateless-replay compatibility.

Only `GET /api/cli/models` is functional. `POST /api/cli/chat` reads a bounded
body and returns JSON HTTP 501, **not** an SSE stream. It intentionally does
not yet decode the Atuin protocol or call any provider. Other paths return
JSON 404; unsupported methods on known routes return 405.

The port is configurable; the host is not. The listener is always IPv4
`127.0.0.1`. Client authentication is absent, and Authorization headers are
ignored. Do not expose this port with a proxy, SSH tunnel, port forward or
container port publication. Loopback does not protect against other local
processes/users or browser-origin attacks; this is a personal local process,
not a multi-user security boundary.

At most four chat requests are admitted, including body reads, with no waiting
queue. Excess requests return 503 with `Retry-After: 1`. Each admitted body is
limited to 4 MiB (including chunked bodies); larger bodies return 413. Headers
have a 16 KiB configured limit and a five-second read deadline; full request
reads have a 15-second deadline. Idle HTTP connections expire after 60 seconds.
The HTTP write deadline is 310 seconds. Future provider work must enforce the
separate 60-second inactivity and 300-second generation timeouts, 1 MiB SSE
events, 256 KiB tool arguments, and 32 calls per response; those limits are
constants only until those layers exist.

SIGINT/SIGTERM closes admission, allows active handlers up to five seconds to
drain, then cancels handler contexts and forcibly closes connections. Startup
and failures use JSON logs on stderr. Request bodies, headers, configuration
contents and upstream credentials must never be logged. Configuration failures
are intentionally generic to avoid parser diagnostics exposing secret values.

Run `make check` for tests, race detector, vet, pinned staticcheck and
vulnerability scanning; `make build` creates the binary. Phase 1 has no parser
to fuzz yet; bounded fuzzing starts with the phase 2 decoder. No live OAuth
checks are run in CI.
