# Go rewrite implementation plan

## 1. Goal

Build a small, personal, localhost-only Go server for a developer machine,
including a remote machine used over SSH, that:

- works with an unmodified Atuin CLI running on that same machine using
  `endpoint_protocol = "oss"`;
- binds only to that machine's loopback interface (`127.0.0.1`) in v1;
- implements the two Atuin AI routes and SSE protocol documented in the
  [Atuin AI server API specification](../specs/atuin-ai-server-api.md);
- uses native Go Responses-format inference against the Codex backend, never
  Chat Completions;
- targets ChatGPT Business subscription usage through a native Go device login
  compatible with installed Pi 1.1.0's `openai-codex` provider;
- supports local and SSH-based development without an OAuth callback listener
  or SSH callback forwarding;
- preserves the CLI/server local-tool boundary;
- has bounded concurrency and memory; and
- ships first as one small local Go executable.

There is no client-facing authentication in v1. The loopback-only listener is
the security boundary. Configurable bind addresses, allowed-host policies,
TLS/proxy deployment, containers, Kubernetes, and client Bearer authentication
may be added later as one reviewed feature rather than partially implemented
up front.

This is a source-informed rewrite, not a formal clean-room implementation. The
reference server and core are Apache-2.0; the Atuin client is MIT. We may port
test cases, fixtures, prompts, or tool descriptions where useful, provided the
repository retains the appropriate copyright/license notices and records
provenance.

## 2. Authentication decision and remaining validation

V1 selects the Pi-style Codex compatibility adapter described in
[ADR 0004](../decisions/0004-pi-style-codex-authentication.md), superseding
ADR 0001's independent SIWC browser-only choice for v1. Implement login and
inference directly in Go, not through a Codex app-server, Pi subprocess, agent,
or harness. Codex is the direct model backend; Atuin still owns local tool
execution and the server owns only one generation per request.

### Selected native device flow

Use installed Pi 1.1.0's `openai-codex` OAuth and Responses implementations as
source references, pinning their package versions, source hashes, and provenance
before implementation. The reviewed registration is Codex's fixed client ID
`app_EMoamEEZ73f0CkXaXp7hrann`, not an independently registered SIWC client.

1. Request a user code from
   `https://auth.openai.com/api/accounts/deviceauth/usercode`.
2. Show the code and `https://auth.openai.com/codex/device` activation URL.
   The user can approve from another machine's browser; no SSH callback tunnel
   or local callback listener is required.
3. Poll `https://auth.openai.com/api/accounts/deviceauth/token` with bounded,
   cancellable polling. Exchange the returned authorization code and PKCE
   verifier at `https://auth.openai.com/oauth/token`, using the device redirect
   URI `https://auth.openai.com/deviceauth/callback`.
4. Persist this application's own token/account record and refresh through
   `/oauth/token` with the fixed registration.
5. Send Responses-format streaming requests by default to
   `https://chatgpt.com/backend-api/codex/responses`, with Bearer access token
   and `chatgpt-account-id` header from the corresponding account record.

This explicitly supersedes the former prohibition on the Codex client ID and
private backend **only for this reviewed compatibility adapter**. Do not read
or modify the user's Pi/Codex credential files, reuse their existing login,
collect browser cookies, or scrape credentials. Use an isolated application
credential store and an explicit login for this application.

This is not an endorsement of official independent-client SIWC support. The
private backend and fixed registration are volatile integration dependencies;
source pinning, safe failure behavior, and live validation remain required.
Existing success through Pi is not proof that this Go adapter works.

### Deferred independent SIWC/public API alternative

Independently registered SIWC browser Authorization Code + PKCE remains the
preferred documented public-API alternative for a later release, not a v1 gate.
That distinct flow uses dynamic registration, scopes including `offline_access`,
`resource.invoke`, and `chatgpt.tokens.use.direct`, rotating refresh tokens,
and public `/v1/models` and `/v1/responses`. Its Responses preview requirements
and tool namespace rules must not be silently applied to Codex requests.

Official SIWC references (for that future integration, not Codex support):

- <https://developers.openai.com/siwc/token-sharing-open-source>
- <https://developers.openai.com/siwc/token-sharing-open-source/sign-in>
- <https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference>
- <https://developers.openai.com/siwc/token-sharing-open-source/self-hosted-vms>
- <https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations>

The current live independent SIWC attempt reached naming consent with “Use
plan”, then failed under organization policy with
`3p_delegated_access_policy_denied`. No tokens or streamed inference proof
were achieved. This is not completion of Phase 0, nor proof that all SIWC
accounts are unsupported.

Keep OAuth behind a credential interface and backend-specific request
projection separate from Atuin handling. Codex, future SIWC, and optional
API-key public Responses requests are distinct wire contracts, not
interchangeable credential substitutions. Native API-key public Responses is
an optional, separately billed fallback and cannot satisfy subscription-login
gates.

## 3. Initial scope

### Included in v1

- `GET /api/cli/models`
- `POST /api/cli/chat`
- a fixed loopback-only listener with no client-facing authentication
- model aliases and default-model selection
- Atuin request decoding and normalization
- OpenAI Responses projection and SSE parsing
- text streaming
- `suggest_command`
- all seven capability-gated local tools
- parallel local tool calls
- multi-request tool continuation driven by the Atuin CLI
- token usage in `done`
- native Pi-style Codex device login, isolated token storage, and refresh
- native Codex Responses-format inference (no agent/harness subprocess)
- optional separately billed public API-key Responses development/fallback mode
- deterministic fake-provider tests and real-CLI tests

### Explicitly excluded from v1

- independently registered SIWC browser/OIDC/DCR login (deferred alternative)
- Codex app-server or Pi subprocess integration
- Pi/Codex credential-file import, cookie collection, or credential scraping
- configurable/non-loopback listening and allowed-host policies
- client-facing Bearer authentication, TLS, and reverse-proxy deployment
- containers, Kubernetes, systemd, and multireplica coordination
- arbitrary server-side tools
- `web_search` and `web_scrape`
- MCP execution
- `/api/cli/usage` and hosted credit accounting
- accounts, multitenancy, billing, or quotas
- server-side transcript/session persistence
- `previous_response_id`-only continuation
- Chat Completions
- arbitrary custom upstream headers/body fields
- compatibility with every historical Atuin version

The pinned client currently tries to execute every non-`suggest_command`
`tool_call` locally, including nominal server tools. Excluding server tools is
therefore a correctness decision, not merely schedule reduction.

## 4. Compatibility decisions

1. The pinned source revisions in the [API specification](../specs/atuin-ai-server-api.md) are the initial
   contract.
2. Compatibility means request/response and workflow compatibility, not
   identical model prose or reproduction of every reference bug.
3. Chat remains stateless. The client sends the relevant transcript on every
   request.
4. One Atuin chat request produces one upstream Responses generation in v1.
5. The CLI executes local tools and opens the next Atuin request with results.
6. Provider function calls are buffered until `response.completed` before
   being emitted to the CLI. This prevents execution of a partial call from a
   subsequently failed response.
7. A batch mixing `suggest_command` and ordinary local tools is rejected
   before any tool is emitted.
8. Responses `call_id`, not output-item ID, is used as the Atuin tool-call ID
   and for later `function_call_output` correlation.
9. The server sends complete history and does not assume Atuin `session_id`
   is a Responses continuation ID.
10. Initially support only models proven to work with stateless history replay.
    If opaque reasoning items are required, design bounded encrypted provider
    state separately rather than silently dropping required state.
11. V1 listens on `127.0.0.1` only and has no Atuin-client authentication.
    Any non-loopback listener is a later security feature, not a config toggle
    exposed by the initial implementation.

## 5. Proposed repository layout

```text
cmd/atuin-ai-server/
    main.go
    serve.go
    auth.go

internal/config/
    config.go
    validate.go
internal/atuin/
    types.go
    decode.go
    normalize.go
    estimate.go
    errors.go
    sse.go
internal/catalog/
    catalog.go
internal/prompt/
    prompt.go
internal/tools/
    registry.go
    schemas.go
    validate.go
internal/safety/
    commands.go
internal/turn/
    runner.go
    reducer.go
internal/provider/
    provider.go
    events.go
    errors.go
internal/provider/openai/
    client.go
    projection.go
    request.go
    stream.go
internal/credentials/
    source.go
    manager.go
    store.go
    oauth.go
    refresh.go
internal/httpserver/
    server.go
    models.go
    chat.go

testdata/compat/
testdata/responses/
testdata/oauth/
tests/integration/
tests/e2e/
tests/resource/

docs/decisions/
docs/authentication.md
docs/compatibility.md
docs/operations.md
config.example.toml
LICENSES/
NOTICE
```

Do not create speculative extension registries. Keep interfaces narrow and add
packages only when they contain real behavior.

## 6. Core interfaces

```go
type Provider interface {
    Stream(ctx context.Context, req ProviderRequest, sink func(ProviderEvent) error) (Usage, error)
}

type CredentialSource interface {
    AccessToken(ctx context.Context) (string, error)
}

type CredentialStore interface {
    Load(ctx context.Context) (CredentialRecord, error)
    Save(ctx context.Context, record CredentialRecord) error
    Delete(ctx context.Context) error
}

type EventSink interface {
    Emit(ctx context.Context, event AtuinEvent) error
}
```

Rules:

- `Provider.Stream` succeeds only after an explicit `response.completed`.
- `response.failed`, `response.incomplete`, premature EOF, malformed calls,
  cancellation, and timeout are failures.
- A sink/write failure immediately cancels and closes the upstream request.
- The HTTP handler is the only owner of downstream SSE writes.
- The turn runner is the only owner of lifecycle transitions and terminal
  emission.
- OpenAI credentials are never exposed through the localhost Atuin API.

Use injected clocks, randomness, HTTP transports, and credential stores for
deterministic tests.

## 7. Implementation phases

### Phase 0 — repository and decision records

Create:

- Go module and standard project tooling;
- an explicit Apache-2.0 project-license decision for user confirmation;
- `NOTICE`/third-party license records;
- ADR 0004 for native Pi-style Codex authentication, explicitly superseding
  the prior SIWC-only v1 choice and recording backend compatibility risk;
- ADR for stateless Responses continuation;
- pinned Atuin/client/server/core revision manifest; and
- compatibility fixture manifest with provenance.

Selected-backend live gates (passed by the isolated Phase 0 probe below),
before building the full server:

1. This application's native Go device login obtains an isolated credential
   record for the intended ChatGPT Business account.
2. That credential → Codex Responses endpoint → completed streamed text on at
   least one intended model.
3. The same model returns a function call and completes a subsequent full-history
   request containing `function_call_output`, without `previous_response_id`.
4. Refresh and durable replacement-token persistence work with that credential.

Pin the installed Pi 1.1.0 OAuth/provider source and capture redacted request,
response, account-header, tool-shape, model-selection, and completion evidence.
Do not assume public `/v1/models` is the Codex model-discovery contract. Verify
which model IDs can be configured/advertised by our local catalog.
Never commit tokens, device codes, or raw authorization URLs.

Record the independent SIWC naming-consent/organization-policy denial as a
blocked alternative, not a passed OAuth or inference gate. Existing Pi login
is not evidence for these gates. Stateless replay and opaque reasoning-state
compatibility remain design gates per ADR 0002; if required state cannot be
preserved, mark the model unsupported or obtain a separately reviewed bounded
encrypted-state design before enabling it.

**Exit:** native device login, completed Codex text, multi-request tool
continuation, and refresh are demonstrated on an intended model with this
application's own credentials. These gates passed in the isolated native Go
probe on `gpt-6.1-sol`; see [redacted live evidence](../proofs/codex-phase0.md).
Production integration remains unimplemented. No success is inferred from
installed Pi, and the independent SIWC denial is not counted as a pass.

### Phase 1 — bounded HTTP shell and configuration

Implement:

- a `serve` command;
- familiar TOML configuration and `CHAT_CONFIG`;
- a fixed `127.0.0.1:<port>` listener (port configurable, host not
  configurable in v1);
- no client-facing authentication middleware;
- ordered model aliases and a default alias;
- model endpoint;
- chat endpoint placeholder;
- graceful shutdown and structured safe logging.

Initial resource limits:

- 4 MiB maximum Atuin request body;
- 4 concurrent chat requests;
- no unbounded waiting queue;
- 1 MiB maximum provider SSE event;
- 256 KiB accumulated arguments per call;
- 32 calls per provider response;
- 60-second upstream inactivity timeout;
- 300-second generation timeout.

Do not expose a host/bind-address option in this phase. Tests must assert that
the listener is on IPv4 loopback and is unreachable through non-loopback
interfaces.

**Exit:** black-box tests cover routes, model order/default, body limits,
saturation, loopback-only binding, unknown routes, and shutdown.

### Phase 2 — Atuin protocol decoder

Implement:

- string and content-block messages;
- text, tool-use, inline tool-result, and remote tool-result blocks;
- legacy assistant `tool_calls`;
- strict role/content validation;
- lenient context/config fields;
- legacy plus nested capability union;
- model alias fallback;
- UUID validation and UUIDv7 generation;
- remote-result fallback text;
- context, user-context, and skill truncation semantics;
- 180,000-token four-characters-per-token estimate;
- transcript consistency validation.

Use `json.RawMessage` for arbitrary tool input so JSON numbers are not
unintentionally converted through `float64`.

**Exit:** ported request/config/capability cases pass and decoder fuzzing cannot
panic or exceed configured work bounds.

### Phase 3 — prompts, tool registry, and safety

Implement model-facing definitions for:

- `suggest_command`
- `read_file`
- `edit_file`
- `write_file`
- `atuin_history`
- `atuin_output`
- `execute_shell_command`
- `load_skill`

Expose each local tool only when its capability is advertised. Keep stable
ordering. Preserve exact field names, required fields, enum values, and bounds
from the compatibility specification.

Start with v1 `atuin_history` behavior. The pinned core ignores
`client_v2_atuin_history`; add v2 only after tests with clients that use it.

Implement a command-suggestion danger backstop that can raise risk but never
lower it. Clearly document that it is a heuristic, not a sandbox.

Prompts and descriptions should be separate from protocol code and covered by
goldens. We may port Apache-licensed reference text with attribution or write
new text; choose explicitly and record provenance.

**Exit:** every capability combination and tool schema has snapshot tests;
malformed and mixed terminal tool batches are rejected before emission.

### Phase 4 — Responses request projection

Project normalized Atuin history to Responses input:

- system policy → `instructions` or documented developer-message form;
- user and assistant text → message items;
- assistant `tool_use` → `function_call` using the same `call_id`;
- user tool result → `function_call_output`;
- failed tool result → deterministic textual error representation;
- full relevant transcript included on every request;
- `store: false`, `stream: true` in the Codex adapter, matching pinned source;
- Codex endpoint, Bearer token, and matching account header.

Validate Codex instructions, tool declarations, returned names, continuation
items, and required reasoning state in Phase 0. Do not silently import SIWC
namespace rules or assume Codex, independent SIWC preview, and public API-key
wire shapes are interchangeable. Keep any optional public API-key projection
separate; future SIWC projection must enforce its own documented restrictions.

**Exit:** an `httptest.Server` verifies exact outbound requests for text,
suggestions, single/parallel tools, failures, and multi-turn continuations.

### Phase 5 — Responses SSE parser

Implement bounded parsing for:

- arbitrary TCP/chunk boundaries;
- split UTF-8 sequences;
- LF and CRLF;
- comments/heartbeats;
- multiline SSE `data` fields;
- text deltas;
- function call item and argument events;
- interleaved parallel calls;
- usage;
- completion, failure, incomplete, and refusal paths;
- unknown additive events.

Keep provider output-item IDs separate from function `call_id`. Reconcile
argument deltas with final snapshots without duplication. Never use the Go
`bufio.Scanner` default token limit without overriding it with an explicit
bounded policy.

Buffer executable tool calls until successful provider completion. Text may
stream as it arrives, but no automatic retry is allowed after any downstream
content has been emitted.

**Exit:** fixture and fuzz tests pass for every-byte splitting, malformed
frames, duplicate events, truncated EOF, oversized frames, timeouts, and
cancellation.

### Phase 6 — Atuin turn coordinator and SSE writer

For each request:

1. validate upstream credential availability and decode before streaming
   (there is no client-facing authentication);
2. choose/generate session UUID;
3. set SSE headers and session header;
4. emit `status(processing)`;
5. emit `status(thinking)`;
6. stream text deltas;
7. on successful provider completion, classify and validate the whole tool
   batch;
8. emit one of:
   - text only → `done`;
   - local calls → `tool_call+`, `status(waiting_for_tools)`, `done`;
   - one suggestion → `tool_call(suggest_command)`, `done`;
9. on post-stream failure, emit one generic terminal `error`;
10. on disconnect, cancel the upstream request immediately.

Enforce exactly one terminal state internally. Unknown/provider-hosted/server
calls must never be emitted to the CLI.

**Exit:** sequence goldens, real-TCP tests, slow-client tests, and cancellation
tests pass. There is exactly one upstream generation per Atuin request.

### Phase 7 — native Codex OAuth credential lifecycle

Implement the Pi-style device flow established in Phase 0 natively in Go:

- the reviewed fixed Codex registration and device endpoints from Section 2;
- bounded user-code acquisition and polling with pending/`slow_down` handling,
  expiry, denial, malformed responses, cancellation, and transport timeouts;
- authorization-code/PKCE-verifier exchange with the device redirect URI;
- account-ID extraction and validation for the Codex account header (claim
  decoding alone must not be described as OIDC signature verification);
- access-token expiry handling and serialized rotating refresh;
- atomic persistence of replacement tokens before allowing further use;
- local logout/deletion and remote revocation only where verified supported;
- explicit reauthentication on `invalid_grant`.

Credential record includes:

- schema version and provider/registration identity;
- account ID bound to the token set;
- access and refresh tokens;
- issue/save/expiry times; and
- monotonically increasing local generation.

Do not copy Pi's credential store or expose raw token responses in errors.

Store atomically in the user's local configuration/state directory with owner-
only directory/file permissions (`0700`/`0600`). An OS keyring integration is
useful when available, but "encrypted" storage with the key beside the
ciphertext is not a security improvement.

V1 supports one local process and one credential writer. Use a lifetime file
lock and stop the server before a separate login/logout command modifies the
record. Multireplica and remote-host coordination are out of scope.

Use an application-specific state directory, never the user's Pi/Codex paths.
Print the activation URL and user code only to the interactive login user;
never log/persist device codes or pending authorization responses. Local and
SSH users approve through a browser without a local callback or tunnel.

**Exit:** fake-device/token-server tests cover pending, `slow_down`, expiry,
denial, cancellation, malformed responses, PKCE exchange, account/header
binding, expiry/skew, concurrent refresh, rotation, missing replacement refresh
token, restart, invalid grants, local logout, supported revocation, atomic-write
failure, permissions, symlinks, corruption, and lock contention. A live opt-in
test covers this application's login → text and tool continuation → refresh →
restart → inference → logout.

### Future alternative — independent SIWC lifecycle (not a v1 phase gate)

If revisited after organization policy/access allows it, separately validate
independent SIWC dynamic client registration, stable `ext_agent_host_id`,
browser Authorization Code + PKCE S256, state/nonce and OIDC verification,
granted scopes, public models/inference, and SIWC-specific tool namespace and
continuation shapes. Direct loopback and SSH-forwarded browser callbacks,
OIDC attack tests, and DCR/host-ID persistence tests belong to that future
feature, not the native Codex device implementation. The current policy denial
must not be recorded as successful SIWC authorization, inference, or refresh.

### Phase 8 — real CLI and differential compatibility

Build an unmodified Atuin binary from the pinned revision and test it with:

- isolated `HOME`/XDG paths;
- `endpoint_protocol = "oss"`;
- temporary Atuin databases;
- a sandbox working directory;
- a deterministic fake Responses service; and
- PTY automation for the TUI where necessary.

Cover:

- model list/default/selection;
- plain text;
- `suggest_command`;
- every enabled local tool;
- permission denial and execution failure;
- parallel tools;
- session continuation;
- expired/revoked upstream OAuth and reauthentication behavior;
- provider error after streaming begins;
- disconnect/cancellation;
- history search and output when daemon support is available.

Also run the original reference container against a deterministic fake Chat
Completions backend and this server against a semantic-equivalent fake
Responses backend. Send identical Atuin requests, normalize UUIDs, usage, JSON
property ordering, and text-delta segmentation, then compare Atuin event
sequences.

Known reference ambiguity—such as its unknown-model test accepting either 200
or 400—must be replaced with an explicit project decision, not copied.

**Exit:** pinned real CLI suite and differential contract suite pass.

### Phase 9 — local binary and resource hardening

Ship:

- a local Go executable for the developer's current OS/architecture;
- reproducible binaries for the primary Linux and macOS targets where
  practical;
- owner-only local configuration and credential files;
- optional loopback-only profiling in explicitly enabled development builds;
- no non-loopback management or profiling endpoint; and
- graceful signal handling with bounded final cancellation.

Container, Kubernetes, systemd, TLS proxy, and remote-host packaging are
explicitly deferred until there is a concrete need.

Initial measurable targets on a documented Linux amd64 environment:

- idle working set/RSS after warmup: goal ≤32 MiB, hard gate ≤64 MiB;
- four normal concurrent streams: goal ≤128 MiB;
- four near-limit admitted requests: hard gate ≤256 MiB;
- no monotonic goroutine, connection, transcript, or heap growth after 100
  completed/cancelled cycles;
- cancellation closes upstream work promptly.

Use `GOMEMLIMIT` as a GC target, not as an RSS guarantee.

**Exit:** local-binary smoke tests, load tests, leak tests, and memory reports
pass.

## 8. Reusing upstream tests

### What can and cannot be reused directly

The original tests are Gleam, Elixir, and Rust. They cannot be linked directly
against a Go implementation. They are nevertheless highly reusable as:

1. behavioral specifications to port into Go table tests;
2. language-neutral JSON/SSE fixture sources;
3. scenarios for differential black-box tests; and
4. an oracle exercised by running the upstream reference container in the
   test environment (this does not make containers a deployment requirement).

Do not mechanically translate every test. Port the compatibility-relevant
cases, strengthen weak assertions, and write new Responses/OAuth tests.

### High-value source inventory

| Source | Tests | Reuse plan |
|---|---:|---|
| `atuin-ai-server/test/atuin_ai_server_test.exs` | 6 E2E tests | Port model-list, full chat stream, and sanitized upstream-error tests as real TCP tests. Strengthen unknown-model behavior. Web transport and web-tool registration are out of v1 scope. |
| `atuin-ai-core/test/.../http/request_test.gleam` | 19 | Port nearly all message/block, legacy call, estimate, and remote hydration cases. |
| `atuin-ai-core/test/.../domain/config_test.gleam` | 23 | Port nested/legacy capability, malformed-field fallback, alias, context, skill, truncation, and gating cases relevant to standalone behavior. |
| `atuin-ai-core/test/.../domain/capabilities_test.gleam` | 8 | Port all recognized/unknown/merge/round-trip cases. |
| `atuin-ai-core/test/.../domain/models_test.gleam` | 5 | Port alias/default/visibility behavior that remains in scope. |
| `atuin-ai-core/test/.../domain/tools_test.gleam` | 3 | Port capability ordering and tool exposure cases; add complete schema goldens. |
| `atuin-ai-core/test/.../domain/safety_test.gleam` | 9 | Port as baseline, then add cases that expose/reference known heuristic gaps. |
| `atuin-ai-core/test/.../domain/prompt_test.gleam` | 3 | Reuse only if reference prompt behavior/text is intentionally adopted. |
| `atuin-ai-core/test/.../domain/usage_test.gleam` | 13 | Port provider-neutral usage aggregation/missing-data cases that map to Responses. |
| `atuin-ai-core/test/.../engine/loop_test.gleam` | 25 | Port text, suggestion, client tools, mixed-batch rejection policy, failures, partial usage, disconnect, duplicate/late events, and terminal invariants. Skip or rewrite server-tool iterations. |
| `atuin-ai-core/test/.../engine/stream_test.gleam` | 8 | Port provider-neutral stream assembly/error invariants. |
| `atuin-ai-core/test/.../llm/openai_compat/stream_test.gleam` | 13 | Reuse fragmentation, heartbeat, parallel-call, malformed-frame, and usage ideas—not Chat Completions wire fixtures. |
| `atuin-ai-core/test/.../llm/openai_compat_test.gleam` | 16 | Port transcript repair and tool exchange semantics, rewriting expected output for Responses. |
| `atuin-ai-core/test/.../llm/openai_endpoint_test.gleam` | 22 | Mostly not reusable because it specifies Chat Completions. Keep general URL/auth/error cases only if applicable. |
| `atuin-ai-core/test/.../http/driver_test.gleam` | 2 | Port appended tool-exchange semantics. |
| OpenRouter/Fireworks recorded fixtures | 11 files | Do not use as Responses correctness fixtures. They are useful only as a malformed/fragmentation corpus or reference-server oracle input. |
| Atuin client `fsm/tests.rs` | large | Use as the behavior oracle for immediate local-tool handling, suggestion termination, continuation, cancellation, and unknown tools. Do not attempt a wholesale Rust-to-Go port. |
| Atuin client `tui/state.rs` tests/behavior | n/a | Use its transcript encoding as canonical request fixtures. |
| Atuin resume/render tests | many | Out of server scope except selected end-to-end resume smoke tests. |

The core currently has 259 named Gleam tests. Only a focused subset is needed
for v1; a target of roughly 80–120 ported compatibility cases plus new
Responses/OAuth tests is more valuable than a mechanical port of all 259.

### Tests that must be newly written

No upstream suite covers the intended architecture. Add original tests for:

- Responses request projection;
- Responses SSE event types and interleaving;
- item ID versus call ID;
- `response.failed`/`incomplete`/premature EOF;
- Codex endpoint, account-header binding, request fields, and full-history
  text/tool continuation matching the pinned adapter;
- backend-specific projection separation from optional API-key and future SIWC
  contracts (no assumed SIWC namespace rules in Codex);
- native device polling, pending/`slow_down`, expiry, denial, and cancellation;
- device authorization-code/PKCE exchange;
- isolated credential storage, locking, token rotation, and persistence;
- stateless replay and rejection of models needing unavailable opaque state;
- backpressure and slow Atuin clients;
- client disconnect propagation;
- body/event/argument/concurrency limits;
- Go memory/goroutine/connection retention; and
- an unmodified real Atuin CLI.

### Fixture format

Prefer language-neutral cases such as:

```json
{
  "name": "assistant tool call without content",
  "request": {"messages": []},
  "expected": {
    "status": 200,
    "events": [
      {"event": "status", "data": {"state": "processing"}},
      {"event": "status", "data": {"state": "thinking"}},
      {"event": "tool_call", "data": {"id": "call-1", "name": "read_file"}},
      {"event": "status", "data": {"state": "waiting_for_tools"}},
      {"event": "done"}
    ]
  }
}
```

Each imported/derived fixture records:

- upstream repository;
- commit SHA;
- original file/test name;
- license;
- whether it is copied, transformed, or independently recreated; and
- intentional expected-behavior differences.

## 9. CI and release gates

Required on every change:

```sh
go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
govulncheck ./...
```

Run bounded fuzz smoke tests in normal CI and longer fuzzing nightly. Run the
reference-container differential suite and pinned real-CLI suite in CI where
Docker/PTY support is available. Containers are used only as a test oracle.
Live OpenAI tests are opt-in and must never
run for untrusted pull requests.

A release is blocked if:

- any unsupported tool can reach the CLI;
- a tool call can be emitted twice;
- executable calls are emitted before provider success;
- provider EOF is treated as success;
- a successful writable stream lacks exactly one terminal event;
- credentials appear in logs, fixtures, errors, or status output;
- cancellation leaves upstream work or goroutines behind;
- refresh rotation is not durably persisted;
- memory gates fail; or
- native device login, Codex text/tool continuation, or refresh gates remain
  unproven for the intended model; or
- existing Pi login, blocked SIWC consent, or optional separately billed API-key
  mode is being presented as completion of this application's ChatGPT OAuth
  feature.

## 10. Suggested agent work breakdown

Keep one owner for the turn state machine and protocol decisions. Parallelize
only packages with stable interfaces:

1. **Agent A:** repository scaffold, config, and loopback-only HTTP shell.
2. **Agent B:** Atuin decoder/normalizer and imported compatibility corpus.
3. **Agent C:** tool schemas, prompt policy, safety checks.
4. **Agent D:** Responses projection and fake provider.
5. **Agent E:** Responses SSE parser and fuzz fixtures.
6. **Agent F:** native Codex device OAuth, refresh, and isolated credential
   store after Phase 0 is settled.
7. **Agent G:** real CLI PTY harness, differential oracle, resource tests.
8. **Integrator:** turn reducer, event ordering, cancellation, and final review.

Do not have multiple agents independently invent lifecycle rules. Land the
Atuin types, provider-event types, and reducer invariants before parallel
implementation begins.

## 11. Definition of done

The project is complete when:

- an unmodified supported Atuin CLI can list models and complete text,
  suggestion, and all local-tool workflows;
- native Go inference uses the validated Codex Responses adapter directly,
  with no Codex app-server or Pi subprocess;
- native device login works with the target ChatGPT Business account using
  this application's own isolated credentials, including refresh and restart;
- local and SSH-based login need no callback forwarding and do not access
  the user's Pi/Codex credential stores;
- Phase 0 text, tool-continuation, refresh, and stateless/opaque-state model
  gates pass; optional separately billed API-key mode is not a substitute;
- pinned compatibility sources and private-backend volatility are documented;
  deferred independent SIWC is not represented as implemented or proven;
- all ported compatibility, new Responses/OAuth, differential, and real-CLI
  tests pass;
- cancellation/backpressure/resource limits are proven;
- the local executable meets the memory gates; and
- operations, security boundaries, supported client/model versions, and known
  differences are documented.
