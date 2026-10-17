# Third-party and source provenance

[`upstream-sources.json`](upstream-sources.json) pins the client, server and
core contract from the existing API specification. Each record identifies the
repository, immutable commit, license path and retained license SHA-256.
Pinned local git trees and their license blobs were inspected on 2026-10-16;
there is no claim that HEAD/latest upstream was used. The core is the server's
v0.4.0 baseline described in the specification.

Phase 3a adds the exact [native Codex discovery source pin](codex-discovery.md),
retained upstream license/NOTICE, independently authored protocol/cache/tests,
and [redacted live evidence](../proofs/codex-discovery-phase3a.md). No raw model
catalog or credential material is retained.

## Current inventory

| Material | Use | Terms / attribution |
| --- | --- | --- |
| Reference server and core | Source-informed protocol/plan research; no code imported in Phase 0 | Apache-2.0; full unmodified license text retained in `LICENSES/Apache-2.0.txt` (identical in both pinned trees) |
| Atuin client | Source-informed protocol/plan research; no code imported in Phase 0 | MIT; full license and Copyright (c) 2021 Ellie Huxtable retained in `LICENSES/Atuin-MIT.txt` |
| OpenAI SIWC public documentation | Summarized/linked research in auth ADR/checklist, not vendored | No redistribution license inferred; not covered by the Apache/MIT source records |
| Phase 2 decoder tests/fixtures | Independently recreated request/config/capability behavior, plus original boundary tests; no upstream implementation copied | Apache-2.0; per-case [fixture manifest](../../testdata/compat/manifest.json), attribution in `NOTICE` |
| Phase 3 prompts, descriptions, schemas and safety | Original prose and implementation; source-informed schemas/behavior and independently recreated baseline cases | Apache-2.0; [Phase 3 checkpoint](../plans/phase3-status.md), per-case manifest, attribution in `NOTICE` |
| Bubblewrap 0.11.1 | External test/build filesystem/PID namespace prerequisite; no source/binary vendored or server linkage | LGPL-2.0-or-later; [package/source/checksum and scope](phase8-tooling.md); historical HOME/XDG-only runs are not isolation evidence |
| Phase 8 real CLI/oracle fixtures and build recipe | Independently recreated synthetic behavior; transformed oracle-only Dockerfile, no app source changes | [Per-case manifest](../../testdata/compat/phase8-manifest.json), [exact tooling/artifact provenance](phase8-tooling.md), MIT/Apache attribution in `NOTICE`; no new Go dependency |
| Installed `@earendil-works/pi-ai` 1.1.0 | Source-only research for native Codex auth/inference decision; no implementation copied by the decision | MIT; artifact paths/hashes and future import requirements in [ADR 0004](../decisions/0004-pi-style-codex-authentication.md) |

This is a reference-source inventory, not a dependency SBOM. The Go module's
runtime dependencies are recorded in [dependency notices](dependencies.md). No root NOTICE was present in the
three pinned upstream trees; this says nothing about notices in future import
paths or transitive dependencies. Apache license boilerplate is not a basis
for inventing an upstream copyright holder. Preserve any actual file headers
and applicable attribution when importing material.

## Required import process

Before copying or adapting code, prompts, tool descriptions, tests or fixtures:

1. Select an immutable commit and exact path/test; inspect its actual license,
   file headers and applicable NOTICE records (including nested/vendored work).
2. Retain required copyright, license and attribution notices. For modified
   Apache material, add prominent modification notices; do not imply endorsement.
3. Record destination, repository/commit, original path/test, license and
   copied/transformed/independently-recreated classification. Record intentional
   behavior changes; independently recreated cases still need their source basis.
4. Update `NOTICE`, the source inventory, and fixture manifest as applicable.
   Keep third-party licensing distinct from project-license confirmation.
5. Scrub secrets and personal data before import, review the diff, and only then
   treat the material as available test coverage. Chat Completions recordings
   are not Responses correctness evidence.

See the [accepted project-license decision](../decisions/0003-project-license-proposal.md)
and root [LICENSE](../../LICENSE) for project-owned material. The files under
`LICENSES/` retain third-party license texts; their original terms still apply.
