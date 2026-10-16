# Phase 3a checkpoint — discovered catalog, not production login

Date: 2026-10-16. Implemented bounded/testable runtime behavior; **live eligible
catalog exit NOT PASSED**. No Phase 4 work, generation, SSE or production OAuth
lifecycle is included. This is not release ready.

## Implementation

- `internal/provider/codex`: native authenticated catalog fetch (not public
  `/v1/models`), projected metadata only; 2 MiB/256-model/string bounds,
  eight-second fetch deadline, no redirects or raw upstream error bodies.
- `internal/credentials`: minimal injected atomic provider/account/reauth-epoch
  credential source. No credential store or production refresh implementation.
- `internal/catalog`: private immutable snapshots, copy-on-projection, upstream
  IDs as aliases, picker visibility and replay allowlist, validated override,
  unknown-ID resolution to the same snapshot's default. Only `gpt-6.1-sol`
  remains eligible. Ascending numeric priority and deterministic ID tie-breaks;
  prefer eligible `-sol` family. Source does not establish model release
  timestamps or numeric slug version semantics, so the documented priority
  fallback is used, **not guessed model-version ordering**. Synthetic selection
  tests do not expand the production allowlist.
- Cache freshness is 15 minutes; validated same-identity stale data may serve
  only transient failures at age <= one hour. Auth/metadata/override failures
  discard it. Account/provider/reauth changes and explicit invalidation cancel
  old fetches and prevent publication. Concurrent refreshes coalesce; one
  caller's cancellation does not cancel others, and the last waiter cancels
  shared upstream work. Clocks/credentials/transport are injected in tests.
- `internal/httpserver`: one shared catalog source for model projection and
  per-chat decoder resolution. Valid injected chat still returns 501. Standalone
  executable has no production credential source, so models/chat return 503
  rather than fake models. Body/admission limits remain intact.
- Optional config accepts only `port` (default 17391) and `default_model`.
  Obsolete model tables/mappings produce migration advice; no manual catalog.
- `cmd/codex-proof discover`: opt-in native bounded isolated live probe with
  fixed-schema safe diagnostics; no raw metadata or credential output.

## Evidence, provenance and blocker

Exact OpenAI Codex commit and SHA-256 pins, retained license/NOTICE and design
choices: [provenance](../provenance/codex-discovery.md).
[Live evidence](../proofs/codex-discovery-phase3a.md): HTTP 200, required schema
field/type subset decoded, one model, no replay-proven `gpt-6.1-sol`. Historical
`schema_validated:true` did not establish full metadata enum/ID/bounds/duplicate
validation; this claim is corrected in the evidence, and shared full metadata
validation plus malformed-report regressions are now implemented. No new live
probe was run to replace that historical result. No stale credentials or
refresh/login workaround was used. The eligible-catalog gate is **NOT PASSED**;
no unproven replacement was enabled. ETag and release recency metadata are not
claimed verified.

Coordinator-authorized bounded follow-up: one `prove-discovered` invocation
selected at most one actual visible candidate and failed before text proof
completed. No alternate candidates/retries, local tool execution, raw metadata
retention or eligibility expansion occurred. The exact initial failure cause
was not retained; no unsupported inference is made from it. The live gate
remains **NOT PASSED**. [Follow-up evidence](../proofs/codex-discovered-replay-followup.md).

## Validation

- `make check`: PASS (Go tests, race tests, vet, staticcheck v0.7.0,
  govulncheck v1.1.4 with no vulnerabilities; five-second fuzz smoke targets
  `FuzzDecode`, `FuzzValidateBatch`, `FuzzEscalate`).
- `make build`: PASS.
- `git diff --check`: PASS.
- Live isolated `discover --approve-live`: expected failure, exit 1, counters
  recorded above. This is a blocker, not a passing automated/live gate.
- Bounded live `prove-discovered --approve-live`: exit 1; no text/tool/replay
  proof passed. Follow-up unit tests exercise one-candidate selection,
  priority/Sol fallback, single-attempt behavior and failure redaction.

Tests replace static catalog assumptions and cover numeric priority 2/10,
Sol preference/no-Sol fallback/ties, overrides, visibility, empty/unproven
catalogs, malformed/oversized metadata, bounds, HTTP statuses, cancellation,
coalescing, exact TTL/stale bounds, immutable projection, hard-error invalidation,
provider/account/reauth/logout isolation, in-flight invalidation and no-config
unwired startup. All automated runtime tests use injected credentials/catalogs
and local fake HTTP, never the retained probe session.

## Independent-review fixes

- Hard refresh failures advance result invalidation revision, including already
  completed transient/stale flights with delayed consumers. The failing flight
  still owns its exact safe error. A controlled regression schedules stale
  completion, newer unauthorized/metadata/override failure, then old consumption.
- Both success and error from credential lookup are checked against captured
  lifecycle revision under the cache lock. A delayed obsolete lookup error
  cannot erase a newly authenticated account's validated snapshot.
- Native metadata validation (IDs, visibility, duplicates and string bounds) is
  separate from eligibility and shared by fetch, selection and candidate proof.
  Diagnostic tests distinguish valid-but-ineligible from malformed metadata.
- Configuration is read once, bounded to 64 KiB; key/migration and typed decode
  use the same immutable bytes. Snapshot/replacement and size tests were added.
- HTTP tests use a changing/counting source and a private observer to establish
  one source read per request and observe decoder fallback resolution. No new
  public test API or generation implementation was added.

`make check`, `make build`, and `git diff --check` pass after these fixes. No
further live probes or eligibility changes were performed. Independent final
review cleared all five fixes and the bounded `prove-discovered` extension,
including `internal/codexproof/discovered_replay.go` (`ProveDiscovered`,
`proveDiscovered`, `selectReplayCandidate`) and `cmd/codex-proof/main.go`.
It attempts only one candidate, aborts on first failure, withholds its ID on
failure and never updates eligibility automatically. Independent verification
also passed `make check`, `make build`, `git diff --check`, and
`go test -race ./internal/catalog -count=20`.

## Handoff / remaining work

This independently reviewed implementation checkpoint is cleared for authorized
normal delivery to `main`; exact commit identity and CI outcome are recorded in
the delivery report rather than embedded in the commit itself. Under user
authorization, Phase 4+ can proceed against injected credentials/providers after
the exact-SHA delivery checkpoint while the live eligibility gate remains open. Phase 7
must publish lifecycle source changes and synchronously invalidate dependent
caches before completing logout/account-change/reauth operations; epochs change
on reauth even to the same account, not ordinary token refresh. It must complete
production login-only initialization, secure store/refresh/logout and startup
integration. Current safe 503 behavior is intentional, not a completed login-only
experience. This is not a full Phase 3a exit or release-readiness claim.
