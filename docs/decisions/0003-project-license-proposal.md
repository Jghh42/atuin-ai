# ADR 0003: Adopt Apache-2.0 for project-owned material

- Status: accepted — explicitly confirmed by the user
- Date: 2026-10-16

## Context

This is a source-informed rewrite, not a clean-room claim. The reference server
and core are Apache-2.0; the pinned Atuin client is MIT. Their licenses do not
automatically license this repository's original material.

## Decision

Adopt Apache License 2.0 for project-owned code and documentation. The user
explicitly approved the proposal: “Lets go with apache 2 as suggested.”
The full license is retained in the root `LICENSE`. Project attribution uses
“atuin-ai contributors”; no individual legal name or exclusive ownership is
inferred from a GitHub handle.

Keep third-party material under its original terms and preserve required
notices. This decision does not relicense upstream MIT material or grant rights
to material outside the project's control. Existing third-party references and
the TOML dependency are recorded in `NOTICE`, `LICENSES/`, and the provenance
records; no upstream implementation or compatibility fixtures have been copied.

## Alternatives considered

MIT was considered. Apache-2.0 was selected for alignment with server/core reuse
and its explicit patent grant.

## Consequences

The project-license confirmation gate is cleared. Distributions must include
the project license and applicable third-party notices. Future imports still
require a per-file license/provenance review. This decision does not clear the
separate selected-backend live validation gate or authorize further
implementation phases. SIWC was later deferred by ADR 0004.
