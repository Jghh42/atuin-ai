# Architecture decision records

This directory contains architecture decision records (ADRs) for choices that
materially affect compatibility, security, deployment, or provider behavior.

Planned initial records:

1. Upstream authentication and the supported ChatGPT sign-in flow.
2. Stateless Responses transcript replay and reasoning-state constraints.
3. Compatibility baseline and supported Atuin versions.
4. Credential persistence for a single-replica self-hosted deployment.

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
