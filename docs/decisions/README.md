# Architecture decision records

This directory contains architecture decision records (ADRs) for choices that
materially affect compatibility, security, deployment, or provider behavior.

Recorded Phase 0 decisions:

1. [SIWC browser auth with loopback or SSH forwarding](0001-upstream-authentication.md)
   — superseded for v1; SIWC deferred after organization policy denial.
2. [Stateless Responses transcript replay](0002-stateless-responses-continuation.md)
   — accepted design; isolated Codex tool replay passed on `gpt-6.1-sol`,
   production integration and other-model/SIWC validation remain separate.
3. [Apache-2.0 project license](0003-project-license-proposal.md)
   — accepted, explicitly confirmed by the user.
4. [Native Pi-style Codex login initially; SIWC later](0004-pi-style-codex-authentication.md)
   — accepted; isolated login, text/tool replay and refresh/restart proof passed;
   the production lifecycle/composition is now the independently reviewed
   [Phase 7 code checkpoint](../plans/phase7-status.md); production live gates
   remain NOT PASSED.

Future records may cover the localhost security boundary, supported released
Atuin versions, and single-process credential persistence. Those are not
implemented or proven by these Phase 0 documents.

Use filenames such as `0001-upstream-authentication.md` and this structure:

```markdown
# Title

- Status: proposed | accepted | superseded
- Date: YYYY-MM-DD

## Context

## Decision

## Alternatives considered

## Consequences
```
