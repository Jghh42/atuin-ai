# Native Codex discovery source pin (Phase 3a)

Inspected https://github.com/openai/codex at exact commit
`96eb400d1231f0199e96f640c740096087cf3e9b` (2026-10-16 retrieval).
The Go implementation and tests are independently authored, source-informed
protocol adaptations; no Rust implementation, fixture, prompt or fallback
catalog was copied. No Codex/Pi binary is invoked or credentials imported.

SHA-256 of inspected upstream files, relative to that immutable commit:

| Path | SHA-256 |
| --- | --- |
| `codex-rs/codex-api/src/endpoint/models.rs` | `b63ebf34bf4ae0490419a4cd976a0e935701d65f3e2be60f000d5fa6b692ad79` |
| `codex-rs/protocol/src/openai_models.rs` | `4b080aeaf5c158104e27d8277f3bc03ee3ad55b55a41c30a788d831fb2e2a99f` |
| `codex-rs/models-manager/src/manager.rs` | `6c1cd0663a27011a05cde5a50a31c6b8cdb1d156e5bc15409bbc062022cdf814` |
| `codex-rs/models-manager/src/lib.rs` | `b31b6e8fdaebedbbf07999b27c02201020e8d876ae293ac05a8fe4bf051244d3` |
| `codex-rs/codex-api/src/auth.rs` | `261a57b4b00489057372e99704bd7e0f6d4a6532e3407343d29cb980ce0d6b05` |
| `LICENSE` | `d17f227e4df5da1600391338865ce0f3055211760a36688f816941d58232d8dc` |
| `NOTICE` | `9d71575ecfd9a843fc1677b0efb08053c6ba9fd686a0de1a6f5382fd3c220915` |

Retained upstream license/notice: `LICENSES/OpenAI-Codex-Apache-2.0.txt` and
`LICENSES/OpenAI-Codex-NOTICE.txt`. Copyright 2025 OpenAI. Upstream NOTICE also
mentions Ratatui; no Ratatui code is used here. See root NOTICE.

## Verified contract and deliberate differences

- `ModelsClient` uses `GET models` under the Codex provider base, appending
  `client_version`. Authentication is applied through the session auth provider.
  Existing Phase 0 pins/proof establish Bearer and matching account headers.
- This application requests
  `https://chatgpt.com/backend-api/codex/models?client_version=0.134.0`,
  identifies itself as `atuin-ai/phase3a`, and sends the isolated account header.
  `0.134.0` is a concrete compatibility query version, not this app's version;
  upstream published tag `rust-v0.134.0` points at
  `39413d891af56ac742fa65ec7b95428d8ddf2ae2`. The contract source pin above is
  distinct. The live request with this query returned HTTP 200 and valid schema.
- `ModelInfo` has `slug`, `display_name`, optional description, `visibility`
  (`list`/`hide`/`none`) and signed integer `priority`. `show_in_picker` comes
  from `visibility == list`; `supported_in_api` restricts API-key mode, not
  ChatGPT mode (`openai_models.rs` model-preset filtering).
- Models manager sorts priority ascending. **No model release date or model
  version field/convention is established.** `ClientVersion` and
  `multi_agent_version` are NOT model release metadata. Neither lexical IDs nor
  `gpt-X.Y` arithmetic proves release recency. Therefore use the plan's
  upstream-priority fallback within eligible `-sol` models, then other eligible
  models, with ID tie-breaks. Tests use priorities 2 versus 10 numerically;
  invented model-version ordering is intentionally absent.
- Only `gpt-6.1-sol` passes our existing stateless replay allowlist. Availability
  is not compatibility proof. Selection helpers test broader synthetic sets,
  but no production configuration can expand this allowlist.
- Unlike upstream, no bundled/static/file catalog or model-ID fallback is used.
  Body/count/string/time limits and safe errors are project-owned. ETag is
  exposed by source but live conditional revalidation has not been proven;
  no ETag support is claimed or enabled. Snapshot freshness/stale policy is
  independently implemented; no disk persistence or raw metadata forwarding.

The private backend is volatile. Source inspection is not live eligibility
proof: see [redacted evidence](../proofs/codex-discovery-phase3a.md).
