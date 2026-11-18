# Operations

How to run, configure and troubleshoot the local server. For first-time setup
use [`init`](init.md); for packaging and installation see [packaging](packaging.md).

## Commands

```sh
atuin-ai-server init [--config PATH] [--atuin-config PATH]   # attended setup
atuin-ai-server serve [--config PATH]                         # run the server
atuin-ai-server auth login|status|logout|diagnose             # server must be stopped
atuin-ai-server token show|rotate                             # local client token
atuin-ai-server version                                       # no config, state or network
```

`auth login` shows the fixed activation URL and user code only on the
controlling terminal (`/dev/tty`); there is no unattended, browser-cookie or
credential-import path. A successful login reports that the bundled catalog is
available; upstream acceptance is not verified until you chat.

`auth status` reports local state only (logged in, logged out or reauthentication
required, plus generation, auth epoch and expiry). It never refreshes or
contacts the network.

`auth diagnose` makes at most one bounded remote model-metadata request with an
already-usable access token and prints a sanitized JSON report to stderr. It
never refreshes, logs in, runs inference or changes state. Share only that
report. See the [report contract](authentication.md#saved-login-model-diagnostics-native-no-refresh).

`auth logout` durably replaces local secrets with a counter-only tombstone. It
does not revoke the session remotely or securely erase disk history.

## Configuration

The server reads the first of: `serve --config PATH`, `CHAT_CONFIG`, then
`$XDG_CONFIG_HOME/atuin-ai/config.toml` (`os.UserConfigDir()` elsewhere). A
missing default file means built-in defaults; a missing explicit file is an
error. The file is read once, up to 64 KiB.

Only two keys are accepted ([example](../config.example.toml)):

- `port`, default **17391**. The host is always `127.0.0.1`.
- `default_model`, exactly `gpt-6.1-sol` (the default) or `gpt-6-luna`.

Obsolete `[[models]]` or alias mappings produce a migration error. Error
messages are deliberately generic so parser output cannot leak secret values.
Create configuration by hand with `umask 077`; the server does not chmod or
rewrite it.

## Models

`GET /api/cli/models` returns Sol first (default) and Luna second without any
remote request. Chat resolves unknown model IDs to the default. Listing still
requires a valid application login; otherwise it returns JSON 503. Bundled
membership is not proof of account entitlement. See the
[catalog policy](catalog-policy.md).

Inference is pinned to the account and auth epoch it started with. A logout,
reauthentication or account change during a turn rejects it rather than
rebinding. Upstream 401/403 fails the turn with no retry or model fallback.

## HTTP boundary

The listener is always IPv4 `127.0.0.1` on the configured port.

- Every request, on every path, needs exactly one
  `Authorization: Bearer <client token>` header; otherwise the server returns
  401 `unauthorized` before routing.
- A `Host` name other than `127.0.0.1` or `localhost` gets 403
  `forbidden_host` (DNS-rebinding defense in depth).
- See [ADR 0006](decisions/0006-required-local-client-token.md) for the threat model.

The token (`aia1_` plus 64 hex digits) is created on the first `serve`,
`auth login` or `init` and stored as the 0600 `client-token` file in the
private state directory. Atuin sends it from `[ai].api_token`; `init` writes it
there when you accept the Atuin edit. `token show` prints it, even while the
server runs. `token rotate` replaces it, needs the server stopped, and requires
updating Atuin's `api_token`. Because the token also lives in Atuin's config,
keep that file private and out of published dotfiles.

Do not expose the port through a proxy, SSH tunnel, port forward or container
port. Anyone with the token can use your upstream account.

Other responses: invalid chat requests return 400 (`invalid_request` or
`conversation_too_large`); oversized bodies 413; unknown paths 404; wrong
methods on known routes 405. See [decoder policies](compatibility.md).

## Limits

| Limit | Value |
|---|---|
| Retained TCP connections | 64 (extra connections are closed immediately) |
| Concurrent chat requests | 4, no queue; extra requests get 503 with `Retry-After: 1` |
| Request body | 4 MiB, including chunked bodies (413 above) |
| Request headers | 16 KiB, read within 5 s; full request within 15 s |
| Idle connection | 60 s |
| SSE frame write and flush | 5 s each |
| Generation | 300 s; 60 s upstream inactivity |
| Upstream SSE frame | 1 MiB, including the final snapshot of the whole response |
| Tool calls | 32 per response, 256 KiB arguments each |
| Estimated input | 180000 tokens |

Each chat makes exactly one upstream generation, never runs tools itself and
never continues automatically. Tool calls are buffered until the whole batch
passes schema, capability, ID-collision and danger checks. Local calls end with
`waiting_for_tools` then `done`. Failures use one fixed generic SSE error, never
raw upstream details. Unknown usage counts and all costs are reported as null.

Upstream connections use HTTP/1 without pooling (at most six per host) and
refuse redirects.

## Private state

State lives in `os.UserConfigDir()/atuin-ai/credentials`: Linux
`~/.config/atuin-ai/credentials`, macOS
`~/Library/Application Support/atuin-ai/credentials`. A symlinked config
directory (common with dotfile managers) is resolved once before use.

- The final directory must be owned by you with mode 0700.
- Files (`lock`, `record.json`, `intent.json`, `client-token`) must be regular
  0600 files with no hard links.
- No ancestor directory may be a symlink, be owned by another non-root user, or
  be group- or world-writable (unless it is a root-owned sticky directory such
  as `/tmp`).

When a check fails, commands name the offending path and the fix, for example
`credential state unavailable: /home/me/.config is writable by group or others;
run: chmod go-w /home/me/.config`. On distributions with umask 002, directories
you created yourself are often group-writable.

State is not encrypted at rest and there is no keyring integration. A hostile
process running as your user, root, network filesystems, Windows and multiple
replicas are outside the security boundary. Never print or copy private state
to debug a problem, and never edit credentials by hand.

### One process at a time

`serve` and every `auth` command take the same non-blocking lock. If the server
is running, other commands fail with `credential state unavailable: in use by
another atuin-ai-server process (usually the running server); stop it first`.
Never delete the lock file to get around this. `token show` reads without the
lock and works while the server runs.

### Token refresh

Before every refresh the server durably records that the old session is about
to be replaced, so a crash can never reuse a refresh token that may already
have been rotated upstream.

- **The request never left the machine** (offline, DNS failure, refused
  connection, proxy or TLS handshake rejection): the old session is restored and
  the next request retries. The current request gets 503.
- **Anything ambiguous** (timeout after connecting, `invalid_grant`, malformed
  reply, failure to save the new tokens): the session becomes unusable, also
  after restart. Run `auth login` again.
- **Saving the intent fails**: no refresh is sent. The running server treats the
  session as unusable, but restarting it restores the old session.

Once new tokens arrive, saving them finishes even if the caller cancels.
Shutdown waits for in-progress refresh and save work before releasing the lock.
Full invariants are in [authentication](authentication.md#rotation-and-restart-safety).

## Reasoning continuation

The normal runtime also keeps optional encrypted private reasoning state in a
`continuation-v1` namespace beside the credentials. `auth status`, `auth
diagnose` and a declined `init` never open it. The local root key is plaintext,
so protect the namespace like credentials. If a conversation's state is lost,
use `/new` in Atuin. Limits, recovery and rollback are in
[reasoning continuation](reasoning-continuation.md).

## Shutdown and logging

SIGINT or SIGTERM stops admission, lets active handlers drain for up to five
seconds, then cancels them and closes connections.

Logs are JSON on stderr. Request bodies, headers, configuration contents and
credentials are never logged.

### Generation failure diagnostics

A failed generation emits one `generation_failed` JSON line with exactly seven
keys: `time`, `level`, `msg`, `phase`, `category`, `upstream_http_status` (null
when there is none) and `elapsed_ms`. No error text, body, header, URL, model or
tool name, ID, prompt, arguments, usage or credential is included. Phases and
categories are closed enums defined in
[`internal/provider/failure.go`](../internal/provider/failure.go); unknown values
become `unknown`. With reasoning continuation, successful turns can also emit
`generation_reasoning_handled` or `generation_reasoning_replayed` with the same
keys.

Categories describe the observed failure branch, not a root cause. Reporting
happens synchronously after the terminal SSE write, so a permanently blocked
stderr can hold a request slot; fix the log destination rather than restarting
in a loop.

When asking someone for help, share only the `generation_failed` lines, never
a whole journal, request dump or credential file.

## Development checks

`make check` runs formatting, tests, the race detector, vet, staticcheck and
nilness, govulncheck, five-second fuzz smoke tests and the packaging, resource
and reasoning harness self-tests. Packaging tests need Python 3.9+, OpenSSL and
curl 8.4.0+, and use only a loopback TLS fixture. Run a longer fuzz campaign
with, for example:

```sh
go test ./internal/tools -run='^$' -fuzz=FuzzValidateBatch -fuzztime=1m -parallel=2
```

The nightly workflow runs each fuzz target for two minutes. CI never performs
live OAuth.

### Compatibility gates

The pinned Atuin CLI end-to-end and reference-oracle differential tests use
synthetic accounts and backends inside Bubblewrap 0.11.1 with Linux user, PID and
mount namespaces. Missing prerequisites fail rather than skip.

```sh
python3 tests/e2e/build-atuin.py /path/to/pinned-atuin /scratch/atuin-build
python3 tests/differential/build-oracle.py /path/to/pinned-server /scratch/oracle
ATUIN_E2E_MANIFEST=/scratch/atuin-build/manifest.json \
ATUIN_ORACLE_MANIFEST=/scratch/oracle/manifest.json make compatibility
```

Provisioning needs network access and Linux amd64 build tools; Docker is needed
only for the oracle. CI runs both gates on every push and reports a
`Phase8 validation aggregate` check. Details are in [compatibility](compatibility.md).

### Resource gates

`make resource-test` checks fixture invariants quickly.
`make resource-isolation-test` needs real network and PID namespaces.
The full gate measures a development package against fixed memory, descriptor,
cleanup and retention bounds (about seven minutes, using the real 60 s and 300 s
timeouts):

```sh
make package-dev VERSION=v0.0.0-dev.local OUT=/abs/fresh-package
make resource ARTIFACTS=/abs/fresh-package OUT=/abs/fresh-evidence
```

Workloads and bounds are recorded in
[resource lifecycle](provenance/resource-lifecycle.md). A release needs all
four CI families green on the exact commit: Go checks, packaging, the Phase 8
compatibility aggregate and the Phase 9b resource aggregate.
