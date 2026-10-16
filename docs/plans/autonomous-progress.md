# Autonomous implementation progress

Updated 2026-10-16. User-authorized sequential work; checkpoints require review
and delivery before moving on. This file distinguishes code from live gates.

| Phase | Status / evidence |
| --- | --- |
| 0 | Historical isolated native login, `gpt-6.1-sol` streamed text/tool full-history replay and refresh/restart PASSED; production integration not implied. [Evidence](../proofs/codex-phase0.md). Independent SIWC alternative remains blocked. |
| 1–2 | Historical bounded shell and decoder completed; static catalog/8080 assumptions now superseded by Phase 3a, not retroactively claimed discovery behavior. [Phase 2](phase2-status.md). |
| 3 | Prompts/tools/safety implemented, no tool emission. [Checkpoint](phase3-status.md). |
| 3a | Independently reviewed implementation checkpoint, cleared for authorized direct-main delivery. Native HTTP discovery/required-field decoding demonstrated, but eligible live catalog **NOT PASSED**: returned catalog omits replay-proven `gpt-6.1-sol`. Injected tests and independent verification pass. Production credential wiring deferred to 7; full phase exit is not claimed. [Checkpoint](phase3a-status.md). |
| 4 | Independently reviewed native Responses request projection/HTTP opening checkpoint, cleared for authorized direct-main delivery. Deterministic injected tests; no parser/Stream implementation or live attempts. [Checkpoint](phase4-status.md). |
| 5–6 | Not implemented. Chat generation stays 501 when a valid injected catalog exists; production remains safe 503. |
| 7 | Not implemented. Must complete isolated production lifecycle, durable refresh, logout/account cache invalidation and no-config login-to-serve experience. |
| 8–9 | Not implemented; real CLI, resource hardening and release gates remain. |

## Phase 3a delivery checkpoint

Delivered on main as `2431b10fe9fb8287de008f0ad0ceacca6f5f096b`.
Exact-HEAD GitHub Actions run `38950960197`: completed **success** (verified
read-only before Phase 4 implementation). Live eligible catalog remains
**NOT PASSED**; the green deterministic CI does not clear that gate.

## Phase 4 validation checkpoint

Request projection only, independently reviewed and cleared for authorized
direct-main delivery. Exact SHA and CI outcome belong in the delivery report.
No Phase 5 parser/state machine, Phase 6 integration, or Phase 7 store work.
`make check`, `make build`, and `git diff --check`: PASS. Full check includes
unit/race tests, vet, pinned staticcheck/govulncheck (no vulnerabilities), and
bounded existing fuzz smoke tests. Results/interfaces/next steps are recorded
in [Phase 4 status](phase4-status.md). Independent review found a dangling
terminal-suggestion replay and credential-lookup cancellation classification.
Both fixes cleared by independent final review: suggestions replay as labeled non-execution assistant
text with original raw arguments, never synthetic outputs; context errors are
preserved immediately after lookup. Added before/after-turn and blocked-source
regressions, and pinned/read Pi's `dist/api/transform-messages.js` dependency.
No new live attempts or eligibility changes; the catalog blocker is still
**NOT PASSED**. Post-fix `make check`, `make build`, `git diff --check`, and
`go test -race ./internal/provider/codex -count=20`: PASS. Independent validation
also passed full check/build and repeated race tests. No new review findings.

## Phase 3a validation checkpoint

- `make check`: PASS: `go test ./...`, `go test -race ./...`, `go vet ./...`,
  pinned staticcheck and govulncheck (no vulnerabilities), plus bounded decoder,
  batch-validator and safety fuzz smoke tests.
- `make build`: PASS.
- `git diff --check`: PASS.
- `go run ./cmd/codex-proof discover --approve-live`: exit 1, **NOT PASSED**
  eligible-catalog gate. Historical safe report: native HTTP 200; required
  field/type decoding succeeded (not full metadata validation); one model;
  integer priority; proven model absent. [Redacted evidence](../proofs/codex-discovery-phase3a.md).

Source pin: OpenAI Codex `96eb400d1231f0199e96f640c740096087cf3e9b`, hashes and
retained license/NOTICE in [provenance](../provenance/codex-discovery.md).
No public `/v1/models`, third-party credential reuse, device-code interaction,
raw logs, static fallback or unproven model enablement occurred. Existing
isolated access token was current; no refresh or new login was performed.

A coordinator-authorized follow-up ran `prove-discovered --approve-live` once
against one automatically selected actual picker candidate, using the same
isolated native credential. It failed before text proof completed (all proof
booleans false); no tool/continuation proof passed, no candidate ID was retained,
and no model was enabled. No retries or alternate candidates were attempted.
The precise initial failure classification was not retained and is not guessed.
The live gate remains **NOT PASSED**. [Bounded replay evidence](../proofs/codex-discovered-replay-followup.md).
Follow-up `make check`, `make build`, and `git diff --check`: PASS.

## Open blockers / decisions

1. Live eligible discovery needs upstream visibility of a replay-proven model
   or separately reviewed replay proof before any allowlist expansion. Do not
   silently treat discovery as tool/stateless compatibility proof.
2. No verified model release metadata/version convention exists in inspected
   source; numeric upstream priority fallback is intentional. ETag revalidation
   is not live verified or implemented.
3. Production startup is port 17391 with safe models/chat 503 until Phase 7
   credentials are wired; no automatic proof-store import. This is not yet a
   user-facing login-only initialization experience or release-ready server.
4. Independent final review cleared all five fixes and the bounded
   `prove-discovered` extension. Independent verification passed `make check`,
   `make build`, `git diff --check`, and `go test -race ./internal/catalog -count=20`.
   Exact commit identity and push/CI outcome belong in the delivery report, not
   inside the commit being identified. Under user authorization, Phase 4+ may
   continue with injected providers after the exact-SHA delivery checkpoint,
   despite the explicitly retained live blocker.

## Independent-review fixes (no further live probing)

- Hard refresh errors now invalidate completed stale-flight results as well as
  the stored snapshot. Controlled regression: stale completion → newer
  unauthorized/metadata/override failure → delayed old consumer rejection.
- Credential lookup success **and error** are checked under lock against the
  captured lifecycle revision. Regression: obsolete delayed error across
  completed reauthentication/new discovery cannot clear the new cache.
- Shared metadata validation is independent from replay eligibility; malformed
  diagnostic tests cover enum, ID, duplicate and string-bound failures. The old
  live `schema_validated:true` claim is explicitly narrowed to required-field
  decoding; full metadata validation is not retroactively claimed live-tested.
- Configuration reads one bounded 64 KiB byte snapshot, eliminating the double
  file-read race; replacement/size regressions pass.
- Counting/changing HTTP catalog tests observe one read per request and decoder
  resolution through a private observer, without public test APIs.
- Post-fix `make check`, `make build`, `git diff --check`: PASS. Live gate still
  **NOT PASSED**. No further live probes were performed.
- Independent final review cleared `internal/codexproof/discovered_replay.go`
  (`ProveDiscovered`, `proveDiscovered`, `selectReplayCandidate`), its tests and
  the `prove-discovered` branch of `cmd/codex-proof/main.go`.

## Later-phase provisioning readiness (read-only inspection)

- Pinned Atuin source is available at `/tmp/atuin-src`, exact HEAD
  `4ca8b77c8a526b8c294eb864a2b74c95c685c983` (verified).
- `rustc`, Cargo and an Atuin executable are not installed/on PATH. Phase 8
  cannot yet claim real-CLI execution; toolchain/binary provisioning is needed.
- Docker daemon is accessible (server 29.7.2). This is test-oracle readiness,
  not a deployment decision or evidence that any reference image was built.
- `/usr/bin/script` provides a PTY; `expect` is absent. Choose/provision a
  bounded PTY harness rather than claiming existing TUI automation.
- Approximately 12 GiB is free on the repository filesystem. Rust builds,
  Docker layers/images and caches need explicit disk budgeting/cleanup; avoid
  unbounded concurrent provisioning or retaining multiple large build trees.
  No toolchain/image provisioning or Phase 8 implementation was done here.
