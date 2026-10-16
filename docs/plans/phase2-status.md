# Phase 2 checkpoint

Phase 2 implements the Atuin decoder/normalizer in `internal/atuin`, wired into
`POST /api/cli/chat` before the generation placeholder.

Implemented:

- string/block messages, text/tool-use/inline/remote results and legacy calls;
- strict roles/content and ID-correlated transcript consistency;
- arbitrary tool input retained as `json.RawMessage` (no float64 conversion);
- lenient advisory context/config, legacy+nested capability union, catalog
  alias/default resolution, standalone prompt-function disabling;
- context/skill prefix limits and exact truncation suffixes, with Unicode
  grapheme-cluster semantics;
- canonical UUID validation, injectable UUIDv7 creation;
- overflow-safe 180,000-token estimate before remote-result fallback hydration;
- body, nesting, structural-work, message and block bounds;
- source-informed request fixtures and config/capability table tests with
  provenance, original edge/boundary tests and decoder fuzzing.

Validation: `make check` (unit/real-TCP tests, race detector, vet, staticcheck,
govulncheck and bounded fuzz smoke) and `make build`. No credentials or live
provider access are needed. See [compatibility](../compatibility.md) for
intentional strictness and [operations](../operations.md) for current HTTP
behavior and longer fuzz commands.

This does not implement Phase 3 tools/prompts/safety, provider projection,
streaming, turn orchestration or production OAuth. Valid chat requests still
return JSON HTTP 501; there is no end-to-end CLI claim yet.
