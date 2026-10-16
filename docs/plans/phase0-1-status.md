# Phase 0 / 1 review checkpoint

## Phase 0: completed under the revised Codex decision

- Go module, local tooling and CI exist.
- Apache-2.0 adopted for project-owned material following explicit user
  confirmation (ADR 0003); root `LICENSE` and third-party notices are present.
- Pinned source/provenance manifests and upstream license texts are retained.
- Fixture manifest honestly contains no imported compatibility cases yet.
- [ADR 0004](../decisions/0004-pi-style-codex-authentication.md) selects native
  Pi-style Codex device login and Codex Responses-compatible inference initially.
  No Pi/Codex subprocess or existing-credential import; no callback tunnel needed.
- Independent SIWC is deferred, not abandoned. Its isolated live probe reached
  naming and plan-usage consent, then returned an organization policy denial
  (`3p_delegated_access_policy_denied`). No successful OAuth exchange or
  inference was demonstrated. Screenshots and diagnostic request IDs are not
  retained in repository evidence.
- **Live selected-backend progress:** fresh isolated Codex device login,
  persisted credentials loaded by a separate process, streamed text and
  full-history synthetic tool continuation passed on `gpt-6.1-sol`. No opaque
  reasoning items were observed in that tool response; this proves only the
  tested transcript/model. See [redacted evidence](../proofs/codex-phase0.md).
- **Refresh/restart passed:** one explicit refresh produced replacement tokens,
  saved atomically as generation 2. Fresh status/inference processes loaded the
  saved record and passed text and tool continuation again. This clears the
  revised Phase 0 exit. No existing Pi credential session was used.

Phase 1's HTTP shell is unchanged and contains no provider/auth implementation.
The isolated probes are research tooling, not production OAuth. The Codex
probe's redacted live results clear Phase 0's integration decision gates only;
production auth/provider lifecycle implementation remains for later phases.

## Phase 1: implemented and locally verified

- `serve`, strict TOML, `CHAT_CONFIG` and explicit/default path handling.
- Fixed IPv4 loopback listener; port-only configuration; no client auth.
- Ordered public model aliases/default; private upstream IDs omitted.
- Chat HTTP 501 placeholder; exact route/method dispatch with JSON errors.
- 4 MiB body bound, four active chats, no application waiting queue, and
  a 64-connection cap before HTTP connection goroutines.
- Header/read/write/idle deadlines, structured safe startup/error logs,
  graceful drain and forced cancellation after five seconds.
- Future provider limits are constants, not claims of an implemented parser.

Independent review found case-insensitive TOML/default ambiguity, unbounded
accepted connections and insufficient shutdown tests. These were fixed and
covered with regression tests. Independent final verification passed.

Validation on Linux amd64 with Go 1.26.9:

- `make check`: tests, race detector, vet, staticcheck and govulncheck pass;
  vulnerability scan reports no vulnerabilities.
- `make build`: passes.
- Binary smoke: models HTTP 200, chat HTTP 501, SIGTERM clean exit 0.
- TCP tests cover non-loopback unreachability, body bounds including chunked
  requests, saturation/recovery, connection admission, graceful drain and forced
  cancellation/connection closure.

At this historical checkpoint Go 1.26.9 was the minimum because the initially
installed 1.26.8 triggered standard-library vulnerability findings, and no
phase 2+ work had started. The current minimum/pin is now Go 1.27.2; see the
[later fuzz-toolchain diagnosis](fuzz-toolchain-diagnosis.md). The validation
environment/results above remain the original Go 1.26.9 evidence.

## Owner review needed

1. **Resolved:** user confirmed the Apache-2.0 project license.
2. **Resolved:** user selected native Pi-style Codex device login/backend
   initially and retained SIWC as a later feature (ADR 0004).
3. **Resolved:** reviewed Codex probe passed isolated login, text/tool replay,
   refresh rotation and post-refresh process restart inference without accessing
   Pi/Codex credentials. Revised Phase 0 is cleared; SIWC is deferred, not passed.
4. Phases 0 and 1 are complete for review. No phase 2+ implementation starts
   without the user's next instruction. Probe credentials are retained privately;
   local cleanup and remote revocation are not claimed as completed.
