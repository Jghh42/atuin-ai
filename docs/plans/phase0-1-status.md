# Phase 0 / 1 review checkpoint

## Phase 0: records ready; exit BLOCKED

- Go module, local tooling and CI exist.
- Apache-2.0 proposal awaits owner confirmation (ADR 0003); no project license
  has been granted.
- Pinned source/provenance manifests and upstream license texts are retained.
- Fixture manifest honestly contains no imported compatibility cases yet.
- ADRs select supported browser/loopback SIWC and stateless full-history replay.
  Native independent-client device authorization is not established.
- **Not run:** independent SIWC login, streamed text, function-call replay,
  Business-workspace catalog capture, refresh rotation/restart proof.
  See [authentication checklist](../authentication.md). No model is proven yet.

Phase 1 was implemented independently of this blocked live gate. It contains
no provider/auth implementation and is not evidence that Phase 0 passed.

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

Go 1.26.9 is the minimum because the initially installed 1.26.8 triggered
standard-library vulnerability findings. No phase 2+ work has started.

## Owner review needed

1. Confirm or revise the Apache-2.0 project-license proposal.
2. Review the shell/configuration and the browser/device decision.
3. Arrange a reviewed, interactive SIWC proof harness/login on the user's
   machine; do not paste tokens or authorization URLs into repository/chat.
4. Clear the live Phase 0 gate before building the provider/auth integration.
