# Documentation

## Specifications

- [Atuin AI server API and compatibility specification](specs/atuin-ai-server-api.md)

## Plans

- [Go rewrite implementation plan](plans/implementation.md)

## Phase 0 records

- [Current authentication decision and validation checklist](authentication.md) — native Codex isolated live proof passed
- [Deferred SIWC checklist](authentication-siwc-deferred.md) and [probe notes](siwc-proof.md) — organization policy denial
- [Compatibility baseline and empty fixture inventory](compatibility.md)
- [Third-party provenance and pinned revisions](provenance/README.md)
- [Apache-2.0 project license](decisions/0003-project-license-proposal.md) — confirmed

The [phase 1 HTTP shell](operations.md) and Go tooling are implemented;
production OAuth and inference are not. An isolated SIWC probe exists but did
not complete authorization. The isolated Codex probe passed login, text/tool replay and refresh/restart;
see [live evidence](proofs/codex-phase0.md) and
[ADR 0004](decisions/0004-pi-style-codex-authentication.md).

## Architecture decisions

Architecture decision records belong in [`decisions/`](decisions/README.md).
Use zero-padded names such as `0001-upstream-authentication.md` and record the
context, decision, alternatives, consequences, status, and date.
