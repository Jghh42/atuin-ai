# Native attended setup (`init`) — implementation checkpoint

`atuin-ai-server init [--config PATH] [--atuin-config PATH]` is the canonical
setup command. There is no `install` alias or service-management CLI. This is
a reviewed DEVELOPMENT delivery checkpoint, **not** a public release or native
LaunchAgent execution receipt. All three final review areas cleared and independent
full acceptance passed sourcee742…; [exact receipts, earlier failures, live-proof
boundaries and pending exact-delivery CI](plans/init-status.md). Documentation
receipt changes create a new holistic delivery identity; earlier artifacts remain
immutable and are never relabeled. Native LaunchAgent/user-systemd execution is
still PENDING and requires the user's own explicit service/config/backup consent. [Exact v258 contract references and
fixture boundaries](provenance/systemd-init.md).

## What happens

1. Open the controlling terminal (`/dev/tty`) for bounded line-oriented prompts.
   Redirected stdin/stdout are not approval channels. Without a controlling
   terminal, refuse **before reading config/state or making changes**. EOF and
   Ctrl-C cancel. Only `y`/`yes` (case-insensitive) approves; every consent defaults
   to **No**. There is no `--yes`, sudo or root-service mode.
2. Open this application's ordinary credential store/lifetime manager. A locally
   active record is retained, including expired access that can normally refresh.
   There is **no refresh, remote catalog, inference or entitlement check**. Missing
   or unusable local login asks `Log in now? [y/N]`. Declining ends setup before
   service/config changes. Approval reuses native Manager.Login/device OAuth;
   the activation URL and code go only to the terminal, never ordinary logs.
   Login cancellation/failure ends setup. Normal `auth` behavior is unchanged.
3. Close the complete runtime and successfully release its lock **before** any
   service start/restart. Preview stable executable, unit path, server config,
   startup semantics and foreground alternative; ask
   `Install and start user service? [y/N]`.
4. Resolve and preview Atuin's config path and the public localhost endpoint;
   ask `Configure Atuin to use the local endpoint? [y/N]`. Ambiguous/unsafe paths
   refuse with manual instructions. Authentication and independently successful
   service setup are retained on config failure; partial completion is explicit.

The server config is still **only** `port` and `default_model`. Resolution is
`--config`, then `CHAT_CONFIG`, then the platform's ordinary application config
path. Explicit files must exist. The bounded validated snapshot and absolute
path are captured once; service setup rechecks the inode/content before use.
If the default file is missing, service consent also authorizes creating that
explicit file with the resolved port, so later supervisor startup cannot silently
select a different config or default. No provider URLs/headers/settings change.

## Lifetime locks and running servers

Only genuine flock contention has the internal typed locked sentinel; permission
or corruption errors do not trigger service probing. Ordinary ready auth does
not probe the supervisor while holding the store. If the store is locked, init
recognizes **only an exact, safely owned generated unit for this setup context**
and verifies the supervisor's loaded fragment/registered path and active process
before offering to stop it temporarily. A same-named systemd unit elsewhere or
foreign registered launchd label is refused, even if the destination file is absent. A changed/foreign
unit, foreground process, different config path/environment, or unknown service
requires a manual stop. Recognition is not proof of which process holds flock:
after stopping the recognized service, init must successfully acquire the normal
lock or refuse. It never kills unrelated processes or unlinks a lock.

Before issuing a consented stop, init records a reconciliation obligation: a
command can apply the stop and THEN fail or be canceled. On decline/cancellation/
failure, previously running managed state is reconciled with a separate bounded
cleanup context only after credential shutdown succeeds and rollback remains safe.
An already running verified service is left alone. Restart also requires the
previously verified PID to be definitively gone (signal-zero observation only;
no terminating signal or name-based process discovery). Unknown/foreign state
is never treated as absence or an authorization to start/stop. Restoration failure is reported without hiding the
primary failure. Failed runtime Close or failed service rollback blocks automatic
restart and requires manual investigation. A config refusal does not undo an
otherwise valid independent service installation.

## Optional user services

Consent covers copying the current executable atomically to
`~/.local/bin/atuin-ai-server` (same bytes = no copy) and installing/starting:

- **macOS:** `~/Library/LaunchAgents/io.github.jghh42.atuin-ai.plist`, a user
  LaunchAgent at **GUI login, not machine boot**. RunAtLoad, restart on unsuccessful
  exit, 10-second throttle, umask 077. No LaunchDaemon or privileged domain.
- **Linux:** `$XDG_CONFIG_HOME/systemd/user/atuin-ai-server.service`, falling back
  to `~/.config/systemd/user`. Enable under `default.target`, start in the USER
  manager, restart on failure with 10-second delay, 3 starts/60 seconds, UMask0077.
  No system unit or automatic linger. A user manager normally starts at login;
  unattended boot persistence requires a **separate operator decision** about
  `loginctl enable-linger USER`, not something init runs or recommends via sudo.

The unit runs the absolute stable executable with `serve --config ABSOLUTE_PATH`,
not a Downloads/scratch executable. HOME and XDG config context are explicit,
INCLUDING the captured default `XDG_CONFIG_HOME=HOME/.config`; a user manager's
different inherited value cannot silently select another application store.
Supervisor commands have a minimal allowlisted environment (Linux runtime directory
and user DBus socket included), fixed absolute executables/verbs, no shell,
unchanged15-second budgets and16KiB TOTAL private stdout/stderr cap. Typed results
separate exit status, startup failure, cancellation and overflow; raw outputs are
redacted and never printed. Cancellation targets only the new owned process group.
No credentials are forwarded as environment variables. XML is escaped; systemd
ExecStart arguments and Environment use separate encoding: dollar signs are
escaped for ExecStart expansion but remain literal in Environment. Both handle
percent/quotes/backslashes and refuse invalid UTF-8/control-bearing paths.

A real active supervisor/process status is required; creating a file is not
success. Linux inspects EFFECTIVE ExecStart/argv/environment, type, drop-ins,
reload-needed state and positive MainPID/ExecMainPID, then verifies that exact
process's UID, executable inode and argv through bounded proc metadata. It never
reads process environment. Overrides, stale/unsupported state and active-without-
process/oneshot units refuse BEFORE stopping or replacing anything; new units
are rechecked after reload before enable/start. Ambiguous systemctl display
representations (including paths whose argv boundaries cannot be proven) require
manual/foreground setup rather than guessed arguments. Only documented v258
empty struct arrays may omit lines: EnvironmentFiles, and ExecStart for not-found
state. Normalization requires a completely successful response; missing required
fields or failed/truncated/overflow responses refuse. Linux's v3 unit uses `@`
to separate the literal executable path from an explicitly dollar-escaped argv[0].
Inspection compares raw v258 SHOW path/flattened LOADED argv (dollars still doubled)
to the exact owned-unit template; runtime proc argv is checked independently
with single literal dollars. Environment's shell-quoted array printer is decoded
separately. Executable paths containing ASCII quotes/backslashes refuse before
mutation because systemd's loader rejects them even when quoted; config arguments
may contain them, and spaces/percent/dollars are supported. [Source-backed
contract](provenance/systemd-init.md). macOS requires exact
registration/program/argv/context and a positive running PID. Only a completed,
nontruncated target-specific not-found result proves launchd absence; generic
errors, missing GUI, timeout and overflow are indeterminate. Failed bootstrap
never authorizes bootout unless the current registration is reverified as ours. Missing systemd user manager, missing GUI domain or tools is a recoverable
unsupported setup, not permission to use root/sudo. Use foreground `serve` instead.

HOME must match the account's OS home, and HOME/config/unit/XDG paths must be
persistent, not `/tmp`, `/run`, macOS temporary folders or a test HOME. Paths must
be absolute/unambiguous with safe nonsymlink ancestry. Sources/destinations are
bounded, euid-owned regular single-link files; executable source and old binary
identity must be this Go application's embedded module identity. An arbitrary
existing executable is never overwritten. Source inode/hash is rechecked, staged
bytes match, files and parent directories are synced. Existing directory permissions
are never chmodded. Only exact generated unit bytes are recognized; changed units
must be migrated/removed manually, rather than guessed or overwritten.

On failure, setup attempts atomic restoration/removal of its own committed file
inodes and previous activation/enabled state. Every applied rename is journaled,
even when a subsequent destination read or directory sync fails: the STAGED inode/
content identity was captured before rename and is not lost on error. Applied,
durable and uncertain outcomes are distinct. Both service files and Atuin config
use best-effort inode/content compare guards before rename/unlink. Detected
replacements refuse, but check-plus-operation is NOT an atomic filesystem CAS;
races after the check remain possible. Hostile same-UID/root processes and
nonlocal filesystems remain outside the trust boundary. Failed
or uncertain rollback is reported separately from the primary failure and blocks
automatic prior-service restoration. A valid independent installation is not
undone merely because the separate Atuin config step refuses. Private
staging files can remain after SIGKILL/power loss; atomic rename does not make
multiple files/supervisor operations one crash-atomic transaction. Same-UID hostile
processes/root and nonlocal filesystems are outside the filesystem trust boundary.

### Start / stop / remove (run yourself, no sudo)

Before separate `auth login|status|logout|diagnose`, stop the running server; these
commands retain their ordinary lifetime lock rule. `init` alone can offer a
recognized managed-service temporary stop. Never remove a lock file.

macOS, in your own GUI-login terminal:

```sh
launchctl print "gui/$(id -u)/io.github.jghh42.atuin-ai"
launchctl bootout "gui/$(id -u)/io.github.jghh42.atuin-ai" # stop/unload
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/io.github.jghh42.atuin-ai.plist" # start
# To uninstall, bootout FIRST, then remove only that reviewed generated plist.
```

Linux, in your own user session:

```sh
systemctl --user status atuin-ai-server.service
systemctl --user stop atuin-ai-server.service
systemctl --user start atuin-ai-server.service
# To uninstall:
systemctl --user disable --now atuin-ai-server.service
# Remove only the reviewed generated unit under ${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user
systemctl --user daemon-reload
```

Uninstalling the service need not remove the stable binary, server config or
credentials. Do not delete a live-login store or config backup to uninstall.
Foreground option (no autostart): `atuin-ai-server serve --config /absolute/server-config.toml`;
stop it with Ctrl-C/SIGTERM before separate auth commands.

## Atuin config resolution and surgical editing

Atuin **18.19** on macOS uses `~/.config/atuin/config.toml` (or
`$XDG_CONFIG_HOME/atuin/config.toml`), **not** Go's macOS UserConfigDir. Official
source `fb33978d9439ba7e97123c1adc932c6a74e3c8ae`, common utils config_dir
lines109–113, has no ATUIN_CONFIG_DIR override in that common function. The
repository's newer pinned Atuin `4ca8b77c8a526b8c294eb864a2b74c95c685c983` source (`atuin-common/src/dirs.rs`,
config_path/try_atuin_home) recognizes ATUIN_CONFIG_DIR, ATUIN_HOME and ~/.atuin.
These are not silently treated as equivalent installed versions.

An explicit `--atuin-config` selects the file. Otherwise, safely inspect deduplicated
existing candidates from absolute explicit env roots, ~/.atuin, XDG and legacy
~/.config. Multiple existing files require an explicit path (even when newer
Atuin would apply precedence). A sole existing candidate is NOT proof it is active:
for example, a legacy file may be ignored when XDG points elsewhere, and18.19 does
not automatically use ~/.atuin. Without an installed-version probe, automatic
selection requires BOTH known18.19 and pinned-newer resolution rules to agree on
that sole existing path. Otherwise (including no existing file/new-file creation),
require `--atuin-config PATH` for the installed version. No existing file is
silently shadowed or edited merely because it is the only candidate. For the
known18.19 macOS setup, explicitly select `$HOME/.config/atuin/config.toml`.

Only these paths are authorized:

```toml
[ai]
enabled = true
endpoint = "http://127.0.0.1:17391" # actual resolved server port, not hardcoded
api_token = ""
```

`ai.model` remains **exactly as written**, including unknown choices or absence.
All other values, comments and sections are retained. Validate UTF-8 BEFORE the
TOML decoder (both UTF-16 BOM byte orders refuse). Bounded64KiB input/output,
string/aggregate-aware statement spans and a NaN-aware semantic before/after
comparison guard the surgical edit; there is no whole-file reencode. Existing
authorized keys must have the expected scalar types (enabled boolean; endpoint/
api_token strings). Invalid aggregates refuse unchanged rather than deleting
comments embedded inside array/inline-table values.
Malformed/duplicate TOML, quoted/dotted/inline AI layouts and explicit non-OSS
endpoint_protocol refuse rather than lose content. Unsupported layouts need a
manual three-key edit, preserving model; Hub protocol must be resolved manually.
No history, sync, permissions, profile or shell file is touched.

Existing safe regular single-link owned targets retain their mode; new config is
0600 in new0700 directories. Symlinks/unsafe ancestors fail with manual instructions.
Changed existing files receive an exclusive same-directory `.atuin-ai-backup-*`
**0600** copy before atomic synced replacement; backups may contain old API tokens
and must stay private. Noop runs create no backup. Inode/content is compared
immediately before commit; detected replacements refuse. This is a best-effort
guard, not atomic compare-and-swap; the same-UID hostile/root/nonlocal-filesystem
exclusions above still apply. Init never prints
config contents, tokens, diffs, provider bodies or account identifiers. On uncertain
post-rename durability, inspect only your own private backup locally; do not share it.

## Manual native Mac acceptance (NOT executed by the writer)

Use the reviewed development artifact/hash and a terminal without recording.
No automatic login/provider gate is implied by these steps. Retain an existing
application login; do not import Pi, proof or another application's credentials.

1. Check `version` and archive checksum; review this document. Run `init` once
   with both optional changes declined. A valid local login must be retained
   without device login, refresh or remote discovery. An expired refreshable
   record must not be replaced just for init. Never share private state/codes.
2. Use `--atuin-config "$HOME/.config/atuin/config.toml"` for the known18.19 file.
   Review paths/semantics; consent to service only if desired. Verify launchctl
   print reports a running PID and localhost models responds normally. Stop/start
   with the commands above, and verify it no longer depends on the downloaded
   executable. This is native execution proof only when the user reports it.
3. Consent to the Atuin change only if desired. Locally check the three keys,
   unchanged model/comments and private backup permissions, without sharing a
   config dump. Repeat init: same file bytes/no additional backup. Check ordinary
   Atuin text/model picker only under separately authorized usage.
4. While the managed service runs, init should offer a temporary stop; decline
   first. Accept on a second run and cancel a later prompt: the previously
   running service should be restored. Foreground/foreign units must only get
   manual-stop instructions; no lock removal. Test stop before separate auth.
5. To decline login without touching real state, use an owned canonical private
   HOME/XDG with no credentials, answer No at login, then stop. Service setup
   from such a temporary HOME is intentionally unsupported. For no-TTY refusal,
   use a detached session, not merely redirected stdin (a terminal may still be
   controlling). Do not capture an attended login or run provider probes for this
   setup test. Linux service execution is a separate native user-manager test.

Synthetic tests and CGO0 cross-builds are **not native LaunchAgent/systemd
execution evidence**. Publication, tags and release authorization remain absent.
