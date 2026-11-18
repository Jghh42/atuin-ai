# ADR 0006: Required local client token

- Status: accepted
- Date: 2026-11-18
- Supersedes: the v1 "no Atuin-client authentication" rule in the implementation plan

## Context

The server listens only on `127.0.0.1` and previously accepted every request.
Loopback is not an authentication boundary:

- any web page can send a simple cross-site `POST` (for example `text/plain`)
  to `http://127.0.0.1:17391/api/cli/chat`, spending the user's ChatGPT seat
  even though it cannot read the reply;
- after DNS rebinding a hostile page is same-origin with the listener and can
  read `/api/cli/models` and full chat responses;
- other local users can connect to `127.0.0.1` on a shared machine.

The unmodified Atuin CLI already sends `Authorization: Bearer <token>` when
`[ai].api_token` is non-empty, and the reference server rejects mismatches
with a fixed 401 body.

## Decision

Every server requires a randomly generated client token; there is no
unauthenticated mode.

- Format: `aia1_` followed by 64 lowercase hex digits (32 random bytes).
- Storage: `client-token`, a 0600 regular file inside the existing private
  credential directory, written by atomic synced replacement under the store
  lock. It is created on first serve, login runtime or init, and never logged.
- Check, before routing on every path: exactly one `Authorization` header equal
  to `Bearer <token>` (constant-time comparison), otherwise HTTP 401
  `{"error":"unauthorized","message":"Invalid or missing bearer token"}` with
  `WWW-Authenticate: Bearer`, matching the reference server.
- Defense in depth: the `Host` name must be `127.0.0.1` or `localhost`,
  otherwise HTTP 403 `forbidden_host`. The port is not compared.
- `init` creates the token while holding the store, before any service starts,
  and writes it as `[ai].api_token` when the user accepts the Atuin edit.
  Otherwise it prints the three `[ai]` lines on the private terminal.
- `token show` prints the token without taking the store lock, so it works
  while the server runs. `token rotate` requires the server to be stopped.

## Alternatives considered

- **Host/Origin checks only.** Blocks browsers but not other local users, and
  depends on browser behavior. Kept only as defense in depth.
- **Optional token.** Leaves anyone who skips `init` exposed.
- **Token in the server `config.toml`.** Config files are often committed to
  dotfiles repositories; the credential directory already has strict
  ownership and permission checks.

## Consequences

- Breaking change for unreleased builds: an Atuin config with an empty
  `api_token` receives 401 until it is updated (`token show` or rerun `init`).
- The token also lives in Atuin's own config, which the CLI requires. Users
  who publish their Atuin config must keep `api_token` out of it.
- Pinned-CLI end-to-end and reasoning harnesses now assert the real CLI sends
  the configured token.
