# Local server / Phase 9a packaging checkpoint

[Binary packaging, installer policy, reproducibility and native Mac checklist](packaging.md).
There is no public release/install endpoint yet. Packaging CI artifacts are
DEVELOPMENT-only and release-blocked; cross-compilation is not native Mac proof.

## Optional attended initialization

Run `atuin-ai-server init [--config PATH] [--atuin-config PATH]` in your own
terminal: local authentication first, optional user-service autostart second,
optional surgical Atuin configuration last. All approvals default No; no TTY
means no state/config mutation. Existing refreshable local login is retained
without network validation. Stable executable is `~/.local/bin/atuin-ai-server`.
macOS uses a LaunchAgent at GUI login (not boot); Linux uses systemd USER,
without enabling linger. Unsupported sessions use foreground serve, never sudo.
See [complete setup, lock/rollback rules, paths, start/stop/removal and native
manual acceptance](init.md). The installer itself does not run setup.

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
# OPTIONAL remote metadata diagnosis only, with serve STOPPED; not a bundled availability test:
./bin/atuin-ai-server auth diagnose # sanitized stderr JSON; NO refresh/inference
./bin/atuin-ai-server serve # config is optional
curl http://127.0.0.1:17391/api/cli/models # valid login required; returns bundled Sol/Luna without metadata GET
# Stop serve before auth status/login/logout in a separate process:
./bin/atuin-ai-server auth logout
```

For identity without configuration, credentials, locks or network, run
`./bin/atuin-ai-server version` (or `--version`). Only the packager injects
reviewable source metadata; ordinary `make build` reports unidentified development.

When manually creating configuration, use `umask 077`, create new directories
0700 and files 0600. Do not chmod/rewrite existing files automatically. Phase 9a
does NOT add config-loader permission enforcement; the credential store's existing
strict checks remain unchanged. The installer never creates/edits either state.

Configuration precedence: `serve --config PATH`, then `CHAT_CONFIG`, then
`$XDG_CONFIG_HOME/atuin-ai/config.toml` (the platform's `os.UserConfigDir` on
other systems). A missing default file uses built-in defaults; an explicitly
selected missing file is an error. Files are read once with a 64 KiB bound;
key validation and typed decoding use the same byte snapshot. Only `port` (default **17391**) and optional
`default_model` are accepted. Remove obsolete `[[models]]`/alias mappings: they
produce a migration error. Overrides must be exactly `gpt-6.1-sol` (default)
or `gpt-6-luna`, not private/public alias mappings or arbitrary IDs.

The executable composes native Codex inference with one lifetime credential
manager and one shared account/epoch-bound bundled catalog. It never imports
isolated proof, Pi or Codex state. `auth login` displays the fixed activation URL
and user code only to `/dev/tty`; no unattended/browser-cookie/import path exists.
Saved login reports bundled availability with upstream acceptance not verified.
Missing valid credentials or an unknown default override gives safe JSON 503.
Both-model native/current Mac/production live gates still block release; see the
[authorized catalog policy](plans/bundled-catalog-policy.md) and
[authentication/security and opt-in steps](authentication.md).

`GET /api/cli/models` returns Sol first/default and selectable Luna second;
chat resolves unknown IDs to that same snapshot's known default. `httpserver.NewWithRuntime` accepts
credentials, that catalog, and the native streaming provider. Catalog-only or
uncredentialed library construction remains safely unavailable, never a
fabricated provider. Invalid decoded requests return 400 (`invalid_request` or
`conversation_too_large`); oversized bodies return 413 before catalog lookup. Other paths return JSON 404; unsupported methods
on known routes return 405. See [decoder policies](compatibility.md).

Normal model listing performs no remote metadata request: no discovery worker,
TTL, stale grace or remote enrichment. Every lookup still preflights application
credentials. Logout/reauth/account change and failed rotating refresh publish
unavailable/new state then synchronously invalidate local snapshots. Lifecycle
revision checks reject obsolete credential lookups. Successful rotation preserves
auth_epoch; fresh login advances it even for the same account. Inference is pinned
to the snapshot's account/epoch and rejects later changes rather than rebinding.
401/403 during inference fails closed after one generation, with no retry, model
fallback or executable partial batch. Bundled membership is not entitlement.
The 180000 estimated-input cap is unchanged. Pi's 272000 context/128000 max-output
metadata is not a simultaneous input-plus-output promise. Luna live proof is
pending; [historical discovery evidence](proofs/codex-discovery-phase3a.md) remains
historical, not a current availability gate.

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
`auth diagnose` opens only EXISTING application state with the same lifetime
lock and strict validation, without directory/lock creation, orphan cleanup or
record/counter changes. It uses only already-usable access: near/expired access
returns `refresh_required` without network; pending/unusable/locked state is
unavailable. Otherwise at most one bounded native metadata GET uses the unchanged
request/validation/selection contract. No OAuth, inference, retry, cache mutation
or eligibility change. One stderr JSON report distinguishes HTTP/transport/body/
schema/metadata/empty/missing-or-hidden-proven/override outcomes with enums,
nullable counters/booleans only. Null means unknown, not zero. Diagnosis completion
exits zero even on a non-success result. Share only the report, never raw responses,
headers, URLs, state, account IDs, tokens or model metadata. See the exact
[field and result contract](authentication.md#saved-login-model-diagnostics-native-no-refresh).
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

Packaging/installer tests within `make check` additionally require Python3.9+,
OpenSSL and **curl >=8.4.0**. They use real curl only against an ephemeral-CA
loopback fixture with an isolated synthetic HOME/rc/netrc, never provider or
release endpoints. Installer/outer bootstrap commands use `curl -q` FIRST to
ignore user rc settings, and old curl fails before any download because older
max-filesize cannot bound unknown-length transfers. [Contract and rationale](packaging.md).

Run `make check` for tests, race detector, vet, pinned staticcheck and
vulnerability scanning plus five-second bounded fuzz smoke tests for the
decoder, tool-batch validator, safety backstop and Responses SSE parser. `make build` creates the
binary. Run longer fuzzing by selecting `FuzzDecode` in `./internal/atuin`,
`FuzzValidateBatch` in `./internal/tools`, or `FuzzEscalate` in
`./internal/safety`, for example:
`go test ./internal/tools -run='^$' -fuzz=FuzzValidateBatch -fuzztime=1m -parallel=2`.
The nightly workflow runs each target for two minutes. No live OAuth checks
are run in CI.

## Offline shipping resource / lifecycle gate (Phase 9b)

`make resource-test` runs fast fixture invariants. `make resource-isolation-test`
requires actual narrow-mount network/PID namespaces and runs synthetic repository
confidentiality, spawn rollback and CONNECT/TLS wire regressions (no real private
files or provider calls). The whole working checkout is NEVER mounted; only
required public inputs checked against the verified package inventory are copied
to a private read-only snapshot. `make resource
ARTIFACTS=/absolute/development-package OUT=/absolute/fresh-evidence` is the
explicit HARD Linux gate (~7minutes, including real60s/300s defaults). Build the
package with the existing `make package-dev` two-path flow first. Missing GNU
time/OpenSSL/Bubblewrap0.11.1 or unusable mandatory network namespaces FAIL,
never skip. No live provider/real credentials, host policy changes, production
knobs or profiling endpoint. The measured shipping binary has default GC and no
race instrumentation/forced GC; numeric companion heap/goroutine/write-error
witnesses are separately labeled. [Exact workloads, bounds, cleanup, reporting
and finite retention resolution](provenance/resource-lifecycle.md),
[review checkpoint and preserved first failures](plans/phase9b-status.md).
Only development package/numeric reports are uploaded by separate resource CI;
synthetic resource success does not clear native Mac/live/catalog/release gates.
Delivery/release MUST independently require ALL FOUR exact-HEAD families: Go
checks, Packaging consistency, Phase8 validation aggregate AND Phase9b resource
validation aggregate. Resource failure/skip/cancellation or a missing/partial
receipt blocks acceptance even if compatibility aggregate is green. Actual
branch required-check settings are external and have not been changed/claimed.

## Synthetic pinned-CLI and differential gates (Phase 8)

These local artifact-gated checks use synthetic accounts/backends. Initial
HOME/XDG-only runs were not ancestor-isolation evidence; see the review record.
Corrected runs require Bubblewrap 0.11.1 with usable unprivileged Linux
user/PID/mount namespaces; missing/restricted prerequisites FAIL, not skip:

```sh
python3 tests/e2e/build-atuin.py /path/to/pinned-atuin /scratch/atuin-build
python3 tests/differential/build-oracle.py /path/to/pinned-server /scratch/oracle
ATUIN_E2E_MANIFEST=/scratch/atuin-build/manifest.json \
ATUIN_ORACLE_MANIFEST=/scratch/oracle/manifest.json make compatibility
```

Use dedicated writable scratch outside the upstream source trees. Provisioning
requires network and Linux amd64 build tooling; the test runs themselves use
synthetic credentials/backends and isolated HOME/XDG/database/sandbox paths.
Python stdlib supplies the actual PTY INSIDE a Bubblewrap filesystem/PID
boundary. Host HOME, /tmp and ancestor context/project/skill files are not
mounted; fixture metadata rejects symlinks/unapproved context before each
launch. Build scripts use fresh complete source/toolchain/Cargo/target inputs,
verify official cached archive checksums on every use and invalidate stale
manifests; local consistency is not independent producer attestation.
Default Atuin features/AI are preserved;
no source patches or private stores are imported. Docker is an oracle-only test
prerequisite; the Go deployment still has no container or new backend/bind knob.
The oracle/fake CC service use an internal bridge with **no published ports**.

`make e2e` and `make differential` run the gates separately and fail without
artifact manifests. Ordinary `go test -v ./tests/...` prints explicit BLOCKED
skips without them; neither a skip nor Go-only CI clears these gates. A separate
PR/push/manual compatibility workflow runs both without live account secrets;
its `Phase8 validation aggregate` depends on Go and actual compatibility jobs,
rejecting failures, cancellations and skips. Administrators must require that
check in branch/release policy; this repository change does not modify GitHub
branch protection. First delivery CI demonstrated hosted namespace/tool and
20-client success, but differential setup and aggregate failed; the focused
Docker fix subsequently passed ALL compatibility jobs/cleanup/aggregate, but
separate Go Checks failed the offline watchdog fixture. The reviewed fixture-only
correction then passed BOTH exact-HEAD workflows at `a50af9a`: 20 CLI / 14 oracle,
namespace/AppArmor probe, cleanup/aggregate and Go make-check/build ([receipt](plans/phase8-ci-receipt.md)). Owned-label watchdog and workflow-always cleanup
remove only the exact run's containers/networks, never global prune.
Keep the manifests/binary/source-built image for independent reproduction, not
as production credentials or packaging. [Full evidence, known differences,
retained initial failures and cleanup policy](plans/phase8-status.md).
Production live/catalog and Phase 9 resource/release blockers remain in force.
