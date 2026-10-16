# Compatibility baseline and fixture provenance

The pinned [source manifest](provenance/upstream-sources.json) and existing
[protocol specification](specs/atuin-ai-server-api.md) define the initial
client/server/core research baseline. These are exact source commits, not a
promise of support for a released Atuin version or all historical clients.
The implementation plan's narrower v1 choices govern the new design: IPv4
loopback only, no client Bearer auth, no server tools, one upstream generation
per Atuin request, full-history replay and rejection of mixed suggestion/tool
batches. Reference-server options described in the specification are not
features implemented here.

## Fixture inventory

[`testdata/compat/manifest.json`](../testdata/compat/manifest.json) deliberately
contains **zero cases**. No upstream tests/fixtures/prompts/tool descriptions
have been imported in Phase 0. There is no passing compatibility suite or real
CLI proof. Planned source inventory in implementation-plan section 8 is a
candidate list, not an import or test result.

For each later case, add a manifest entry with:

- stable `id` and repository-relative `destination`;
- `upstream_repository`, full `upstream_commit`, `original_file` and
  `original_test` (explicitly note if no named test exists);
- `license_spdx`, applicable copyright/notice reference and retained license;
- `reuse`: `copied`, `transformed`, or `independently_recreated`;
- `intentional_behavior_differences` (an explicit empty array when none);
- redaction/modification notes and reviewer verification of provenance.

For wholly original cases, use `reuse: original`, identify their specification
basis and explicitly mark upstream fields not applicable rather than inventing
a source. Do not label a source-informed recreation as original. Keep case
request/expected JSON/SSE separate from this inventory and record every case,
not merely its directory. Normalize volatile UUIDs, usage, JSON ordering and
text segmentation only when those are not the behavior under test.

Before importing, follow the [third-party process](provenance/README.md).
Before claiming model support, complete the selected-backend
[live validation](authentication.md). The isolated Codex proof covers only the
model/transcript documented in [its report](proofs/codex-phase0.md), not real
Atuin CLI compatibility or independent SIWC.
No live data or credential artifacts may become compatibility fixtures.
