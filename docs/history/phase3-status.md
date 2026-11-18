# Phase 3 checkpoint

Phase 3 implements prompt policy, the eight static tool definitions and
pre-emission validation. These packages are deliberately not connected to a
provider or SSE writer yet; valid chat requests still return JSON HTTP 501.

## Implemented

- `internal/tools`: backend-neutral definitions, `suggest_command` first and
  seven v1 local tools in pinned core order, exact capability gating, schemas
  with the pinned field names/required sets/enums/bounds. Returned definition
  bytes are independently owned. No server tools, extensions or execution.
- `internal/prompt`: original concise policy prose, capability and execution-
  preference guidance, escaped untrusted environment/file/skill metadata,
  separate UTC per-turn context and grapheme-prefix limits. No prompt variants,
  dev bypass, unsupported web/docs fetch, upgrade or PTY claims.
- `internal/safety`: original, source-informed heuristic command checks. Matches
  can only raise a suggestion to high danger and append fixed notes, preserving
  the model's assessment and notes otherwise. This is **not** a sandbox or a
  proof of safety; local permission enforcement remains with the CLI.
- `tools.ValidateBatch`: atomic validation of every completed call, including
  capability/name/ID checks, required fields, enums, integer and array bounds,
  duplicate JSON keys, Unicode compatibility, and numeric representability in
  the pinned Rust JSON decoder. Raw numeric lexemes remain untouched.
  Unknown additional fields are allowed as advertised, but must still be valid
  interoperable JSON. Mixed terminal suggestions/local tools and multiple
  suggestions are rejected with no partial batch returned.
- Provider limits enforced at this gate: at most 32 calls, 256 KiB argument JSON
  per call, bounded argument depth/token work. Escalation cannot exceed the
  argument limit. Callers must invoke this gate **only after successful provider
  completion**, then emit only the returned independent batch. Future parser
  and turn-coordinator phases must establish completion/lifecycle guarantees.

## Text and compatibility decisions

Prompt and tool-description prose is newly authored, not copied from core.
Schemas and behavioral test expectations are independently recreated from the
pinned Apache-2.0 source. Safety baseline cases retain source provenance; the
new heuristic deliberately fixes the reference keyword-prefilter blind spots
and is more conservative for recursive deletion and privileged execution.
See [compatibility policies](../compatibility.md), `NOTICE`, and
`testdata/compat/manifest.json` for exact provenance and differences.

## Validation

- Eight tool-definition/schema goldens and a 256-combination capability golden,
  with independent ordering/gating assertions and mutation-isolation checks.
- Three prompt goldens plus execution-preference, capability, untrusted XML,
  Unicode, truncation and UTC-date tests.
- Safety baseline/gap/escalation tests and atomic batch/schema/boundary tests,
  including malformed Unicode and overflowing numbers after a valid call.
- `make check`: tests, race detector, vet, staticcheck, govulncheck and bounded
  decoder, batch-validator and safety fuzz smoke tests.
- `make build`: local executable. Longer fuzz runs are scheduled nightly.

No live inference, credential use, tool execution or real-CLI success is
claimed by these unit/snapshot tests. Phase 4 Responses projection is next.
