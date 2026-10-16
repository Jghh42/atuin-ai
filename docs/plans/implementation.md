# Go rewrite implementation plan

## 1. Goal

Build a small, single-user/self-hosted Go server that:

- works with an unmodified Atuin CLI using `endpoint_protocol = "oss"`;
- implements the two Atuin AI routes and SSE protocol documented in
  [Atuin AI server API specification](../specs/atuin-ai-server-api.md);
- uses the public OpenAI Responses API, never Chat Completions or an
  undocumented ChatGPT backend;
- supports eligible ChatGPT-plan usage through OpenAI's documented Sign in
  with ChatGPT (SIWC) OAuth flow;
- eventually offers a device-code login when there is a supported integration
  path for this application type;
- preserves the CLI/server local-tool boundary;
- has bounded concurrency and memory; and
- ships as one small non-root container image.

This is a source-informed rewrite, not a formal clean-room implementation. The
reference server and core are Apache-2.0; the Atuin client is MIT. We may port
test cases, fixtures, prompts, or tool descriptions where useful, provided the
repository retains the appropriate copyright/license notices and records
provenance.

## 2. Confirmed upstream facts and remaining auth decision

OpenAI currently documents an OSS/self-hosted flow in which an eligible user:

1. dynamically registers an OAuth client;
2. authorizes the scopes `offline_access`, `resource.invoke`, and
   `chatgpt.tokens.use.direct` using Authorization Code + PKCE;
3. receives access and rotating refresh tokens;
4. calls `GET https://api.openai.com/v1/models`; and
5. calls `POST https://api.openai.com/v1/responses` with the OAuth access
   token.

For this flow Responses requests use `store: false`, `stream: true`, and the
full required input history. They do not use ChatGPT's private backend.

Official references:

- <https://developers.openai.com/siwc/token-sharing-open-source>
- <https://developers.openai.com/siwc/token-sharing-open-source/sign-in>
- <https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference>
- <https://developers.openai.com/siwc/token-sharing-open-source/self-hosted-vms>
- <https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations>

### Device-code decision gate

Codex itself implements device login, and current `codex app-server` source has
an `account/login/start` request with `type: "chatgptDeviceCode"`. The SIWC
integration documentation for an independent OSS app, however, currently
documents dynamic registration and browser/loopback PKCE, not direct use of
Codex's client registration or device endpoints.

Before treating literal device login as a release requirement, run a short
spike and choose one supported path:

1. **Preferred:** OpenAI confirms device authorization for dynamically
   registered SIWC OSS clients. Implement it natively in Go.
2. **Acceptable fallback:** use the documented SIWC browser flow. For a remote
   pod, authenticate locally with the same app/account, transfer the protected
   credential record, and let the pod own refreshes, as OpenAI documents.
3. **Optional experiment:** use `codex app-server` as an auth/inference
   subprocess. Proceed only if its tool loop can be disabled and Atuin's local
   tools can be represented safely. This adds a second binary and is not the
   preferred low-memory architecture.
4. **Do not do:** copy Codex's OAuth client ID, parse its private credential
   files, collect browser cookies, or call an undocumented
   `chatgpt.com/backend-api` endpoint.

The Go architecture must not depend on the result of this decision: OAuth is a
credential source behind an interface; the Atuin and Responses layers remain
the same.

## 3. Initial scope

### Included in v1

- `GET /api/cli/models`
- `POST /api/cli/chat`
- optional client-facing static Bearer authentication (`AUTH_TOKEN`)
- model aliases and default-model selection
- Atuin request decoding and normalization
- OpenAI Responses projection and SSE parsing
- text streaming
- `suggest_command`
- all seven capability-gated local tools
- parallel local tool calls
- multi-request tool continuation driven by the Atuin CLI
- token usage in `done`
- SIWC token storage and refresh
- API-key Responses mode as a development/fallback credential mode
- deterministic fake-provider tests and real-CLI tests

### Explicitly excluded from v1

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
    auth.go
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
Dockerfile
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
- Credentials used against OpenAI are entirely separate from `AUTH_TOKEN`.

Use injected clocks, randomness, HTTP transports, and credential stores for
deterministic tests.

## 7. Implementation phases

### Phase 0 — repository and decision records

Create:

- Go module and standard project tooling;
- `NOTICE`/third-party license records;
- ADR for SIWC/browser/device auth;
- ADR for stateless Responses continuation;
- pinned Atuin/client/server/core revision manifest; and
- compatibility fixture manifest with provenance.

Run two live, redacted proof-of-concept requests before building the full
server:

1. SIWC OAuth access token → public `/v1/responses` → streamed text.
2. Same flow with a function call and a following request containing
   `function_call_output`.

Also prove model-list shape and refresh rotation. Never commit tokens or raw
authorization URLs.

**Exit:** direct Responses + tool continuation works for at least one intended
model, and the device-login decision is recorded.

### Phase 1 — bounded HTTP shell and configuration

Implement:

- `serve`, `auth login`, `auth status`, and `auth logout` commands;
- familiar TOML configuration and `CHAT_CONFIG`;
- `AUTH_TOKEN` support;
- ordered model aliases and a default alias;
- secure loopback default binding;
- auth-before-routing behavior;
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

Reject an unauthenticated non-loopback listener unless the operator explicitly
acknowledges that authentication is supplied externally.

**Exit:** black-box tests cover routes, model order/default, all auth cases,
body limits, saturation, unknown routes, and shutdown.

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
- `store: false`, `stream: true` always set in SIWC mode.

Do not send unsupported SIWC preview fields. Validate the current documented
function/custom-tool representation in Phase 0 rather than assuming standard
API-key behavior is identical.

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

1. authenticate and decode before streaming;
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

### Phase 7 — OAuth credential lifecycle

Implement the supported SIWC flow established in Phase 0:

- dynamic client registration;
- stable persisted `ext_agent_host_id`;
- PKCE S256;
- high-entropy state and nonce;
- exact loopback callback validation;
- ID-token signature, issuer, audience, expiry, and nonce validation;
- granted-scope verification;
- access-token expiry handling;
- serialized rotating refresh;
- revocation/logout where documented;
- explicit reauthentication on `invalid_grant`.

Credential record includes:

- schema version;
- issued client ID;
- stable host ID;
- verified account identity;
- ID token when retained for reauthorization hints;
- access and refresh tokens;
- granted scopes;
- issue/save/expiry times; and
- monotonically increasing local generation.

Store atomically in an owner-only directory/file (`0700`/`0600`) on a writable
persistent volume. Do not place a rotating credential file in a read-only
Kubernetes Secret. An optional encryption/keyring layer is useful, but
"encrypted" storage with the key beside the ciphertext is not a security
improvement.

V1 supports one process/replica and one credential writer. Use a lifetime file
lock and stop the service before an external login/logout command modifies the
record. Multireplica refresh coordination is out of scope.

For a remote pod, document local authentication and protected credential
transfer/import. If a supported native device flow is confirmed, add it behind
the same credential manager.

**Exit:** fake-OIDC tests cover callback attacks, state replay, expiry/skew,
concurrent refresh, rotation, missing replacement refresh token, restart,
revocation, invalid grants, atomic-write failure, permissions, symlinks,
corruption, and lock contention. A live opt-in test covers login → inference →
refresh → restart → inference → logout.

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
- invalid and expired authentication;
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

### Phase 9 — packaging and resource hardening

Ship:

- CGO-free Linux amd64/arm64 binary where practical;
- distroless or scratch-derived non-root image with CA certificates;
- read-only root filesystem;
- separate read-only config and writable credential volume;
- optional separate loopback management listener for health/profiling;
- no public profiling endpoint;
- graceful SIGTERM draining with bounded final cancellation.

Initial measurable targets on a documented Linux amd64 environment:

- idle working set/RSS after warmup: goal ≤32 MiB, hard gate ≤64 MiB;
- four normal concurrent streams: goal ≤128 MiB;
- four near-limit admitted requests: hard gate ≤256 MiB;
- no monotonic goroutine, connection, transcript, or heap growth after 100
  completed/cancelled cycles;
- cancellation closes upstream work promptly;
- container memory limit starts at 256 MiB and is revised only from measured
  workloads.

Use `GOMEMLIMIT` as a GC target, not as an RSS guarantee.

**Exit:** non-root/read-only container smoke tests, load tests, leak tests, and
memory reports pass.

## 8. Reusing upstream tests

### What can and cannot be reused directly

The original tests are Gleam, Elixir, and Rust. They cannot be linked directly
against a Go implementation. They are nevertheless highly reusable as:

1. behavioral specifications to port into Go table tests;
2. language-neutral JSON/SSE fixture sources;
3. scenarios for differential black-box tests; and
4. an oracle exercised by running the reference container.

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
- SIWC field restrictions (`store: false`, full input history, unsupported
  fields omitted);
- dynamic OAuth registration;
- PKCE/OIDC verification;
- token rotation and persistence;
- device-flow behavior if supported;
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
Docker/PTY support is available. Live OpenAI tests are opt-in and must never
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
- API-key mode is being presented as completion of the ChatGPT OAuth feature.

## 10. Suggested agent work breakdown

Keep one owner for the turn state machine and protocol decisions. Parallelize
only packages with stable interfaces:

1. **Agent A:** repository scaffold, config, HTTP shell, client auth.
2. **Agent B:** Atuin decoder/normalizer and imported compatibility corpus.
3. **Agent C:** tool schemas, prompt policy, safety checks.
4. **Agent D:** Responses projection and fake provider.
5. **Agent E:** Responses SSE parser and fuzz fixtures.
6. **Agent F:** OAuth/OIDC and credential store after Phase 0 is settled.
7. **Agent G:** real CLI PTY harness, differential oracle, resource tests.
8. **Integrator:** turn reducer, event ordering, cancellation, and final review.

Do not have multiple agents independently invent lifecycle rules. Land the
Atuin types, provider-event types, and reducer invariants before parallel
implementation begins.

## 11. Definition of done

The project is complete when:

- an unmodified supported Atuin CLI can list models and complete text,
  suggestion, and all local-tool workflows;
- the service uses only public `/v1/responses` upstream;
- SIWC OAuth works for a private self-hosted deployment, including refresh and
  restart;
- the documented device-login decision is implemented or explicitly recorded
  as unavailable for independent clients, with the supported headless login
  path shipped;
- all ported compatibility, new Responses/OAuth, differential, and real-CLI
  tests pass;
- cancellation/backpressure/resource limits are proven;
- the container meets the memory gates; and
- operations, security boundaries, supported client/model versions, and known
  differences are documented.
