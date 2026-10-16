# ADR 0003: Propose Apache-2.0 for project-owned material

- Status: proposed — pending explicit user/copyright-holder confirmation
- Date: 2026-10-16

## Context

This is a source-informed rewrite, not a clean-room claim. The reference server
and core are Apache-2.0; the pinned Atuin client is MIT. Their licenses do not
automatically license this repository's original material.

## Proposed decision

Adopt Apache License 2.0 for project-owned code and documentation **only after
explicit confirmation from the user with authority to license it**. Keep
third-party material under its original terms and preserve required notices.
Apache-2.0 is proposed for alignment with server/core reuse and its explicit
patent grant. MIT is a possible alternative, not the chosen project license.

## Consequences and confirmation gate

No project-wide license is granted by this record, `NOTICE`, or the reference
texts under `LICENSES/`. No root `LICENSE`, copyright ownership assertion, or
project SPDX license declaration is introduced. The README must continue to
say that project licensing is unconfirmed.

Before adoption, record the user's approval and scope, establish the authorized
copyright attribution, review any pre-existing contributions and third-party
imports, then add the root license and update metadata/README in a separate
change. Do not retroactively assume permission to license others' material.
Until then, project licensing is unresolved and distribution as an
Apache-licensed project is blocked. Third-party obligations apply regardless.
