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

The [production server](operations.md) now composes native OAuth, discovery and
inference in the independently reviewed [Phase 7 code checkpoint](plans/phase7-status.md),
cleared for authorized direct-main delivery. Fresh production login/live lifecycle and eligible discovery are
**NOT PASSED**. [Phase 8 synthetic compatibility](plans/phase8-ci-receipt.md)
is delivered with both exact-HEAD workflows green; [Phase 9a packaging](packaging.md)
code/local gates are independently cleared for delivery (hosted exact-HEAD CI
pending, no public release), native Mac validation pending,
and Phase 9b resource work unimplemented. An isolated SIWC probe exists but did
not complete authorization. The isolated Codex probe passed login, text/tool replay and refresh/restart;
see [live evidence](proofs/codex-phase0.md) and
[ADR 0004](decisions/0004-pi-style-codex-authentication.md).

## Architecture decisions

Architecture decision records belong in [`decisions/`](decisions/README.md).
Use zero-padded names such as `0001-upstream-authentication.md` and record the
context, decision, alternatives, consequences, status, and date.
