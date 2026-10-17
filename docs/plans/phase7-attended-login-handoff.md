# Phase 7 attended production login handoff — outcomes recorded separately

**Both security-fix re-reviews and independent post-fix verification are CLEARED.**
The coordinator will invite the user to perform fresh login in their own terminal;
this guide does not record or imply an executed flow or live outcome. The user
has already explicitly authorized their own login, its automatic discovery and the initial
serve/models workflow; ordinary bounded catalog reads in that workflow do NOT
require a redundant new approval at each step.
The user is now available; any production outcome must be recorded separately.
Retained-probe success is [separate evidence](../proofs/codex-retained-probe-2026-10-16.md), not a
production or eligible-catalog pass. No credential import, candidate inference,
eligibility expansion or Phase 8 test is part of this login handoff.

## Authorized workflow boundary

The authorized workflow includes fresh explicit production device login, its
automatic post-save catalog lookup, starting `serve`, and the first local
`GET /api/cli/models`. `auth login` is NOT a login-only network operation. A new
server has an empty in-memory cache, so its first models GET causes another
upstream metadata fetch; that ordinary bounded read is included, not forbidden
or subject to a new approval just because the login-time catalog was unavailable.

Unavailable discovery remains a blocked eligible-catalog gate, never success.
Record the login-time and server-time outcomes separately, without polling or
trying to force a pass. This does NOT authorize blind refresh/login retries,
candidate inference probes, eligibility expansion, automatic credential import,
or Phase 8 real-CLI tests. Further live text/tool/refresh/restart/logout proof
remains separately bounded Phase 7 work.

## User's own terminal ONLY — fresh isolated application state

Run these commands yourself on the server machine in a non-recorded controlling
terminal. Do NOT have an agent run `auth login` in a captured tool/PTY session.
Do not use `script`, `tee`, terminal recording, screenshots, command substitution
around login output, or paste the device code into chat. The CLI writes the
activation URL/code to `/dev/tty`; approve it privately in your browser. Never
share tokens, account IDs, state-file contents or the private directory name.

For the coordinator's attended invitation, keep the SAME shell/environment
for every auth/serve command below. This Linux example creates NEW private state
without touching the normal application config or any retained probe/Pi/Codex
session. If any setup command fails, STOP rather than continuing with a default
directory.

```sh
cd /home/pi/code/atuin-ai &&
  set +x &&
  umask 077 &&
  AI_CHECK_CONFIG=$(mktemp -d "$HOME/.atuin-ai-production-check.XXXXXX") &&
  test -n "$AI_CHECK_CONFIG" && test -d "$AI_CHECK_CONFIG" &&
  export XDG_CONFIG_HOME="$AI_CHECK_CONFIG" &&
  unset CHAT_CONFIG &&
  ./bin/atuin-ai-server auth status
```

Use the coordinator-reviewed built binary, rebuilt after the cleared fixes.
No tokens, endpoint override or model list is supplied. Do not copy any credential files into the fresh directory.
The setup chain must complete and `auth status` must say `Logged out`; otherwise
STOP and investigate, without attempting login. Once that setup result is
established in the coordinator-invited attended flow, type this separate command
yourself:

```sh
./bin/atuin-ai-server auth login
```

Only relay the final generic login outcome to the coordinator, NOT the preceding
activation code or terminal contents:

- `Login saved successfully; models unavailable ...`: login/save succeeded,
  but eligible discovery did not. Keep that gate BLOCKED. The already-authorized
  first serve/models check below may still record the server's ordinary metadata
  result; do not loop, retry login or probe candidates to force a pass.
- `Login saved successfully; model discovery available`: report that exact
  outcome and continue with the already-authorized server/catalog check below.
- Any failure: report only its generic message. Do not inspect or send state
  files, and do not blindly retry a rotating session.

## Initial server / projected catalog check

After a saved login, the existing workflow authorization covers this initial
check without another approval:

```sh
# Original terminal, with the SAME XDG_CONFIG_HOME still exported:
./bin/atuin-ai-server serve
```

In a second local terminal on that SAME machine, run ONCE:

```sh
curl --fail-with-body --max-time 15 http://127.0.0.1:17391/api/cli/models
```

Expected success is HTTP 200 and the application's projected `{default,models}`
JSON, with default present among model aliases. The current internal eligibility
allowlist contains only replay-proven `gpt-6.1-sol`; there is no configured model
list, fabricated fallback or authorization to enable other returned models.
A 503 is a blocked eligible-catalog gate, not a reason to poll or probe models.
If port 17391 is already in use, STOP and report it rather than changing bind
settings, forwarding a port or stopping an unrelated process.
Do not call the private upstream catalog directly.

Stop `serve` with Ctrl-C in its terminal and wait for it to exit BEFORE:

```sh
./bin/atuin-ai-server auth status
```

Status is local-only and may report safe generation/auth_epoch/expiry, never an
account ID or token. It does not prove upstream acceptance. While `serve` owns
its lifetime lock, separate auth status/login/logout commands deliberately fail
rather than race it. Do not delete/replace the lock inode to bypass contention.
Do not run logout or delete this isolated state until the coordinator decides
whether to retain it for the remaining authorized production lifecycle proof.
Remote revocation is not supported or claimed.

## Native Mac saved-login diagnostic — retain the existing session

2026-10-17 user report on the actual reviewed darwin/arm64 build at
`76e5b7a6082b1d91923ae24964c53f9208528c13`: version identity correct, fresh
`auth login` saved successfully but reported models unavailable; local status
Logged in, generation 1, auth_epoch 1, expiry date 2026-10-27. These are user
reports, not agent-run live checks. No catalog HTTP status/schema/eligible count
or native inference outcome is established. Historical isolated probe results
must not be attributed to this Mac or its account/session.

That older binary remains valid for its source but does not contain the new
`auth diagnose`; obtain the separately reviewed updated DEVELOPMENT build and
verify its supplied archive hash/version before using the new command. No Go
SDK or Rust/oracle rebuild is needed. Do not guess a download URL or promote an
old artifact by editing its metadata.

STOP `serve` and wait for exit. In the SAME user/environment/HOME used for the
saved login, run ONCE:

```sh
./atuin-ai-server auth diagnose
```

Do not change HOME now or redo login to run this check. We do not assume the
reported login used isolated state. On macOS `XDG_CONFIG_HOME` does NOT isolate
`os.UserConfigDir`: state follows HOME under Library/Application Support.
`CHAT_CONFIG` selects configuration only, not credentials. Necessary context is
only whether HOME was normal/changed, whether the same environment is retained,
and whether default_model or proxy/VPN/custom CA is configured (yes/no; no paths,
config/environment dumps or endpoint values).

The command never calls normal refreshing `Current`: usable existing access
allows one bounded native metadata GET; `refresh_required`, pending/unusable
state, store/lock failure produce NO network. It does not create state/locks,
clean orphan records, change lifecycle counters, log in/out, infer, retry or
change eligibility. Share only the fixed-schema JSON (stderr); unknown fields
are null. A completed diagnosis exits zero even when `result` is not `success`.
See the [field/result semantics](../authentication.md#saved-login-model-diagnostics-native-no-refresh).
No raw body/error/header, account ID, device code, model name/description or
history is printed. No private files or third-party/proof state should be opened
or copied. If metadata is valid but the proven model is absent/hidden, retain
the session and report that blocker; never configure unproven models or poll.
Any follow-up request-contract repair requires source-backed review, not header
or client-version guesses. No native eligible-catalog/inference pass is claimed
until an actual safe result is reported.

## Outcomes still requiring evidence

A saved fresh production login alone does not clear eligible discovery. A 200
projected catalog alone does not clear actual production text/tool continuation,
refresh durability, restart inference or logout. The retained-probe recheck
clears NONE of those production gates. Keep each result separate and retain only
sanitized outcomes; the complete Phase 7 live lifecycle remains **NOT PASSED**.
