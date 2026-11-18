# Documentation

## Using the server

- [Init](init.md): attended setup, user services, Atuin configuration, removal.
- [Operations](operations.md): configuration, limits, private state, shutdown
  and troubleshooting.
- [Authentication](authentication.md): device login, token refresh and the
  credential security boundary.
- [Reasoning continuation](reasoning-continuation.md): optional durable private
  reasoning state.
- [Packaging](packaging.md): release artifacts and the pinned installer.
- [Model catalog policy](catalog-policy.md): the bundled Sol and Luna models.

## Reference

- [Atuin AI server API specification](specs/atuin-ai-server-api.md)
- [Compatibility suite](compatibility.md)
- [Architecture decision records](decisions/README.md)
- [Third-party provenance and pinned sources](provenance/README.md)

## Probes

Opt-in tools under `cmd/` used to validate authentication choices. They are
not part of the server.

- [Native Codex device-login proof](codex-proof.md)
- [Deferred SIWC proof](siwc-proof.md) and [SIWC research checklist](authentication-siwc-deferred.md)

## History

[Phase plans, status receipts and live proofs](history/README.md) record how the
project got here. They are kept for evidence and are not maintained.

## Writing decisions

Architecture decision records live in [`decisions/`](decisions/README.md). Use
zero-padded names such as `0001-upstream-authentication.md` and record the
context, decision, alternatives, consequences, status and date.
