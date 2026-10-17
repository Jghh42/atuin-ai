# Bundled native catalog policy correction — 2026-10-17

## Authorized development policy (supersedes remote availability gating)

The user explicitly approved a Pi-style bundled known catalog and development
selection of Luna. This is an intentional policy change, not a fabricated live
fallback, account entitlement claim, or retrospective clearance of old gates.

Normal runtime exposes exactly `gpt-6.1-sol` (Sol, first and default) and
`gpt-6-luna` (Luna, second, selectable). Upstream IDs remain Atuin aliases.
`default_model` may select either exact ID; any other override fails closed.
Unknown per-chat IDs retain the protocol's known-default fallback. Configuration
still accepts only `port` and `default_model`; manual `[[models]]`/alias mappings
remain forbidden. No flags, upstream URL settings or guessed wire changes.

Every catalog lookup preflights valid application credentials, even for cached
local metadata. Snapshots pin provider/account/auth_epoch through inference.
Logout, failed refresh, reauthentication and account change invalidate local
snapshots synchronously; lifecycle revisions reject obsolete unlocked credential
lookups, including obsolete errors that might otherwise erase new state. Response
slices are defensively copied. Successful token rotation preserves auth_epoch.
There is no discovery worker, TTL, stale remote snapshot or metadata network
dependency. Ordinary credential refresh remains governed by the existing manager.

Native discovery remains **explicit `auth diagnose` only**: one bounded GET,
no refresh, retry, cache publication or credential writes. Its fixed 12-field
report retains historical replay-proof semantics: only Sol has that historical
proof. A Luna-only HTTP 200 still reports `proven_model_absent`; this does NOT
mean the bundled runtime catalog is unavailable. `default_override_valid` is
historical remote selection validity, not bundled override validity. A successful
login says `bundled model catalog available; upstream acceptance not verified`.
No metadata result (absent/hidden/empty/malformed/401/403/429/5xx) disables the
known catalog when stored application credentials are valid. Inference may still
reject either model; 401/403 fails closed after one generation attempt, without
retry, model fallback, executable partial batches or invented completion.

## Metadata and proof boundaries

Public installed Pi AI 1.1.0 built-in data contains both exact IDs with
`contextWindow: 272000`, `maxTokens: 128000`; [artifact hashes and attribution](../provenance/pi-codex.md#bundled-catalog-reference-2026-10-17).
These are reference metadata, not permission, entitlement or a capacity promise.
The existing **180000 estimated-input-token cap is unchanged**. It does not
promise simultaneous 180k input plus 128k output. Original concise descriptions
are used; Luna explicitly says live validation pending. No Pi SDK/subprocess,
third-party credentials, implementation code or user settings are imported.
No automatic remote enrichment is included in this minimal patch.

Historical discovery gating and historical omitted-proven-model evidence remain
truthfully recorded in older phase logs; their availability policy is superseded
here, not their results. New synthetic tests are not live validation. Release
remains blocked on native proof for BOTH IDs, current native Mac execution and
production live lifecycle gates, independent reviews, exact-HEAD CI and explicit
publication approval. The base `7981bb43647dacbab2a4b3d77c07a80cca5c4b98` had all four
CI families green (Go 39015864252, packaging 39015864170, compatibility 39015864214,
resources 39015864201); those receipts do not validate this uncommitted patch.

## Review and independent acceptance receipt

The coordinator reports both independent reviews **CLEARED**: lifecycle/security
and behavior/protocol. Independent complete acceptance **PASS** covers full
checks/build/race x20, actual 20 CLI/14 oracle cases, both-target/two-path packaging
with 36 tests plus 32 mutations, and the entire resource suite with real 60s/300s
defaults: 446 generations, zero catalogs, peak four, all 23 processes cleaned.
Independent HWM KiB: idle 11976, normal 15928, near 127468; unchanged budgets pass.
Setup failures (missing manifest environment/generated two `.pyc` files) were
preserved; only verified generated caches were removed before confirming source.

This acceptance applies to source inventory
`a1af55411682264e09d1004e5ca8c53f8ef57f0c413d10a5fb845c336b2c0b96`.
The independent report `dist/bundled-catalog-independent-resources-2/report.json`
has SHA-256 `ffde9641c55e926467f7d820833a3ff3f1e4fc57e7733c7aace92e9cb36f9dd7`.
Existing development packages/reports remain immutable evidence for that source;
these documentation-only delivery updates do not relabel them as post-update or
committed-source artifacts. New exact-HEAD CI/artifacts are separate evidence.
Development-only commit/push and CI artifact upload are authorized, not a tag,
GitHub Release or production/live acceptance. The historical Mac diagnostic at
`7981bb4` returned valid HTTP 200 metadata without the proven model; it is remote-
only historical evidence, not a test of this bundled catalog. Luna live proof
remains **PENDING** until the separate authorized verifier reports actual results.

## Local validation and verifier handoff

Keep all private sources outside CI, transcripts and artifact directories. Only a
separate authorized verifier may use retained Phase 0 credentials, after independent
code reviews. The writer does not read credentials or execute live inference.
Expected verifier flow: confirm this exact source/build, select Sol then Luna,
perform a few bounded text and harmless synthetic tool/full-history continuation
prompts per ID, verify one generation/request and safe failure behavior, then
report only manually sanitized outcomes. Do not import a proof/Pi session into
production application state, repeat login blindly, broaden model IDs or infer
Mac/production lifecycle proof from isolated retained-session results.

The original pinned CLI remains unmodified; the 20-case suite now actually picks
Sol -> Luna and asserts both native wire IDs/full history. Original-source oracle
configuration is test-only and contains both IDs with matching original metadata;
exact model-list comparison and protocol normalization are unchanged. Source fake
providers validate the chosen model, exact tools and ordered history. Existing
builder/helper recipes are unchanged: cached manifests must still verify, and
changed recipes require genuine fresh builds, never metadata relabeling.

Resource workloads/barriers/default 60s/300s budgets remain unchanged: 446 native
generations, peak four, and **zero automatic remote catalogs**. The old 15 requests
were first-list discovery in 15 fresh process/cache lifetimes; local bundled
metadata removes those GETs rather than replacing workloads or padding counts.
Explicit positive/negative TLS catalog wire tests remain separate and intact.
See [resource provenance](../provenance/resource-lifecycle.md).
