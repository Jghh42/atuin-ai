# Architecture decision records

This directory contains architecture decision records (ADRs) for choices that
materially affect compatibility, security, deployment, or provider behavior.

Recorded Phase 0 decisions:

1. [Independent SIWC browser auth; device login deferred](0001-upstream-authentication.md)
   — accepted design, live validation blocked.
2. [Stateless Responses transcript replay](0002-stateless-responses-continuation.md)
   — accepted design, intended-model/tool proof pending.
3. [Apache-2.0 project-license proposal](0003-project-license-proposal.md)
   — proposed only, explicit user confirmation required.

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
