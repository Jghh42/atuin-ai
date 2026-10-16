# Atuin AI server protocol and implementation notes

This is a reverse-engineered interoperability specification for replacing
[`atuinsh/atuin-ai-server`](https://github.com/atuinsh/atuin-ai-server) while
continuing to use the normal Atuin CLI.

## Scope and source snapshots

The protocol is not published as a versioned OpenAPI or JSON Schema document.
This document therefore describes the behavior of these source snapshots:

- `atuinsh/atuin-ai-server` commit
  [`4d582bc5ceea5b5edfdcf3abb49dc850400cda7c`](https://github.com/atuinsh/atuin-ai-server/tree/4d582bc5ceea5b5edfdcf3abb49dc850400cda7c)
- Its pinned `atuin-ai-core` v0.4.0 commit
  [`565877e97ed3ff4840481b8d47e12eb2f9854039`](https://github.com/atuinsh/atuin-ai-core/tree/565877e97ed3ff4840481b8d47e12eb2f9854039)
- Current Atuin client commit
  [`4ca8b77c8a526b8c294eb864a2b74c95c685c983`](https://github.com/atuinsh/atuin/tree/4ca8b77c8a526b8c294eb864a2b74c95c685c983)

The server repository pins `atuin-ai-core` rather than containing most chat
logic itself. Compatibility should be tested against the exact CLI release(s)
you intend to support because there is no protocol version negotiation.

## Executive summary

A replacement is feasible. The Atuin client has an explicit custom endpoint
mode and does not require an Atuin account for an OSS endpoint.

It is **not sufficient** to expose an OpenAI-compatible endpoint. The CLI talks
to an Atuin-specific JSON/SSE protocol. A minimal compatible implementation
needs:

1. `GET /api/cli/models`;
2. `POST /api/cli/chat`;
3. optional Bearer authentication;
4. the request/message format below;
5. Atuin SSE event names and payloads; and
6. an agent/tool loop that distinguishes tools executed by the server from
   tools executed locally by the CLI.

The reference server is stateless, but it is more than a transparent proxy. It
contains prompt construction, model alias resolution, message conversion,
tool definitions and orchestration, optional web-tool execution, usage
aggregation, safety post-processing, timeouts, and error normalization. The
CLI still owns the UI, transcript/session persistence, context-window
selection, permissions, and execution of local file/shell/history tools.

## Client configuration

```toml
# ~/.config/atuin/config.toml
[ai]
endpoint = "http://localhost:8080"
endpoint_protocol = "oss" # recommended; "auto" also treats non-Atuin URLs as OSS
api_token = "secret"      # optional; omit for an unauthenticated server
model = "llama"           # optional model alias returned by /api/cli/models
```

The CLI appends paths to the configured base URL; it does not replace an
existing path prefix. For example, `https://example.test/atuin-ai` produces
`https://example.test/atuin-ai/api/cli/chat`.

With `endpoint_protocol = "auto"`, official Atuin addresses use Hub login and
usage behavior; other addresses use OSS behavior. Explicit `"oss"` avoids
accidental Hub authentication behavior. An empty `api_token` is treated as no
token.

Source: [client endpoint settings](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-client/src/settings.rs#L648-L685), [endpoint classification](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-client/src/settings.rs#L1318-L1331), and [token selection](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/commands/inline.rs#L47-L65).

## Protocol inventory

The standalone reference server exposes only two application endpoints.
There is no WebSocket, gRPC, health, discovery, session, or usage endpoint.
Unknown authenticated routes normally return HTTP 404 JSON. Authentication
runs before routing, so with `AUTH_TOKEN` set an unauthenticated unknown route
returns 401 instead. All routes pass through the same optional authentication
middleware.

| Method | Path | Request | Response | Purpose |
|---|---|---|---|---|
| `GET` | `/api/cli/models` | none | JSON | Model aliases shown/selectable by the CLI |
| `POST` | `/api/cli/chat` | JSON | SSE (`text/event-stream`) | One agent turn |

The hosted Atuin Hub additionally has `GET /api/cli/usage`, but the standalone
server does **not** implement it and the client skips it for OSS endpoints.

Source: [server router](https://github.com/atuinsh/atuin-ai-server/blob/4d582bc5ceea5b5edfdcf3abb49dc850400cda7c/lib/atuin_ai/server/router.ex#L13-L78) and [client OSS usage behavior](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/context.rs#L14-L29).

## Authentication

### Client-to-server authentication

The reference server reads one static token from `AUTH_TOKEN` at startup.

- Unset: every endpoint is unauthenticated.
- Set: every request must contain exactly one matching header:
  `Authorization: Bearer <AUTH_TOKEN>`.
- Missing, malformed, duplicated, or non-matching authorization produces:

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error":"unauthorized","message":"Invalid or missing bearer token"}
```

There are no users, API-key records, scopes, cookies, refresh tokens, OAuth,
or per-route permissions. Comparison in the reference code is ordinary string
matching, not an expressly constant-time comparison.

The Atuin CLI sends this header when `[ai].api_token` is non-empty. For chat,
a 401 produces a specific missing-token or rejected-token message. Model-list
fetching reports a generic failed-status error. The client only clears
credentials when a rejected chat token came from an Atuin Hub session, which
does not apply in explicit OSS mode.

### Server-to-LLM authentication

This is separate from client authentication. The operator may supply an
upstream key inline, from an environment variable, or from a command's stdout.
By default it becomes `Authorization: Bearer <key>`. Custom request headers
replace that default entirely and can interpolate `{{api_key}}`.

Do not confuse `AUTH_TOKEN` (protects this server from clients) with `api_key`
(authenticates this server to its LLM provider).

## `GET /api/cli/models`

### Request

```http
GET /api/cli/models HTTP/1.1
User-Agent: atuin/<version>
Authorization: Bearer <token>  # only when configured
```

The current client has a 10-second timeout for this request.

### Success response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "default": "llama",
  "models": [
    {
      "alias": "llama",
      "name": "Llama 3.2",
      "description": "Local llama3.2 via Ollama"
    }
  ]
}
```

The current client's deserializer requires top-level `default` and `models`,
and requires `alias`, `name`, and `description` in every model entry. `alias`,
not the upstream model ID, is sent back in chat requests. The reference server
returns models in configuration order. If `default_model` is omitted, the
first configured model is the default.

Project Phase 3a intentionally replaces that reference configuration policy:
upstream discovered IDs are aliases, only picker-visible replay-proven IDs are
advertised, and default selection prefers eligible Sol models using verified
ordering (currently numeric upstream priority, with deterministic ID ties).
`default_model` may override only to an eligible discovered ID. Snapshots are
account/provider/reauthentication-bound, fresh for 15 minutes, and may survive
only transient failures up to one hour since successful validation. Missing
login, empty eligibility or failed initial discovery returns JSON 503, not a
placeholder catalog. Production login wiring remains Phase 7. See
[Phase 3a status](../plans/phase3a-status.md).

Source: [server model response](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/controller.gleam#L63-L100) and [client model types](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/models.rs#L15-L51).

## `POST /api/cli/chat`

A request is one network-level agent turn. It can involve multiple upstream
LLM calls when the server executes server-side tools. When the model requests
a client-side tool, the server ends this request; the CLI runs the tool and
starts a new request containing the expanded transcript and tool result.

### Request headers

The current client sends:

```http
POST /api/cli/chat HTTP/1.1
Accept: text/event-stream
Content-Type: application/json
User-Agent: atuin/<version>
Authorization: Bearer <token>  # only when configured
```

The server extracts the Atuin version from `User-Agent: atuin/<version>` for
feature/trace metadata. There is no explicit protocol-version header.

### Request body

A representative current-client request is:

```json
{
  "messages": [
    {"role": "user", "content": "List the files in this directory"}
  ],
  "context": {
    "os": "linux",
    "distro": "Ubuntu 24.04",
    "shell": "zsh",
    "pwd": "/home/me/project",
    "last_command": "..."
  },
  "config": {
    "capabilities": [
      "client_invocations",
      "client_v1_load_skill",
      "client_v1_atuin_history",
      "client_v2_atuin_history",
      "client_v1_read_file",
      "client_v1_edit_file",
      "client_v1_write_file",
      "client_v1_execute_shell_command",
      "client_v1_atuin_output"
    ],
    "model": "llama",
    "user_contexts": [
      {"path": "/home/me/.context", "data": "..."}
    ],
    "skills": [
      {"name": "deploy", "description": "Deploy this project"}
    ],
    "skills_overflow": "..."
  },
  "invocation_id": "client-generated-id",
  "session_id": "018f7f33-0ad3-7aef-bf0a-d204a18a1234"
}
```

The current client always emits `messages`, `context`, `config`, and
`invocation_id`; it omits `session_id` on the first request and omits optional
config members when unused. The server is more lenient than the current client
shape:

| Field | Wire type | Server behavior |
|---|---|---|
| `messages` | array | The only semantically required field: must decode and be non-empty. |
| `session_id` | string or null | Optional. Missing, null, or `""` generates a UUIDv7. A non-empty value must be a valid UUID. |
| `invocation_id` | string or null | Optional metadata. |
| `context` | object | Optional/advisory; invalid fields become absent and a non-object becomes an empty context. |
| `config` | object | Optional; malformed/unknown fields fall back to defaults. |
| top-level `capabilities` | array | Legacy form, permanently supported by core; unioned with `config.capabilities`. |

Unknown keys are ignored.

### `messages` schema

Messages use an Anthropic-like transcript shape, despite the upstream adapter
being OpenAI-compatible:

```text
Message = {
  role: "user" | "assistant",
  content: string | ContentBlock[]
}

ContentBlock =
  | { type: "text", text: string }
  | { type: "tool_use", id: string, name: string, input: any JSON }
  | {
      type: "tool_result",
      tool_use_id: string,
      content: string,
      is_error?: boolean             // defaults false
    }
  | {
      type: "tool_result",
      tool_use_id: string,
      remote: true,
      content_length?: integer | null,
      is_error?: boolean
    }
```

For backward compatibility, an assistant message can instead carry top-level
`tool_calls: [{id, name, input}]`. It may omit `content` only when that array is
non-empty. Current clients use content blocks.

`remote: true` refers to server-persisted tool output. The standalone server
has no such persistence and always sends server-tool results inline, so a
replacement can omit remote results if it is likewise stateless. It still
accepts incoming remote references, but cannot hydrate them and substitutes
the literal `"[Tool result no longer available]"`. This matters when replaying
a transcript created against a persistent server.

The server rejects conversations estimated above 180,000 tokens, using a rough
four-characters-per-token estimate over content and tool input.

Source: [request decoder](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/request.gleam#L1-L168), [remote-result hydration](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/request.gleam#L187-L218), and [client transcript encoder](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/tui/state.rs#L79-L204).

### `context` schema

All values are optional strings:

```text
context = {
  os?: string,
  distro?: string,
  shell?: string,
  preferred_language?: string,
  pwd?: string,
  last_command?: string
}
```

The current client normally sends `os`, nullable `shell`, nullable `pwd`, and
optionally `distro` and `last_command`. Non-string values decay to absent
rather than rejecting the request.

### `config` schema

```text
config = {
  capabilities?: string[],
  run_preference?: "auto" | "suggest" | "run",
  model?: string,                    // a catalog alias
  prompt_fn?: "default" | "concise",
  user_contexts?: {path: string, data: string}[],
  skills?: {name: string, description?: string}[],
  skills_overflow?: string
}
```

Important reference behavior:

- malformed values are generally ignored/defaulted rather than rejected;
- unknown model aliases fall back to the configured default in the standalone
  deployment;
- at most 10 user contexts are considered; when over length, path retains its
  first 150 characters and data its first 10,000, then the truncation helper
  appends `"... "` plus a suffix (so those are retained-prefix lengths, not
  final lengths);
- at most 50 skills are considered; overlong names retain a 64-character
  prefix and descriptions a 1,024-character prefix, followed by `"... "`;
- overlong `skills_overflow` retains 500 characters followed by `"... "`;
- `run_preference` is effective, although the inspected current client does
  not normally emit it;
- `prompt_fn` is syntactically accepted but then cleared in this standalone
  deployment because it disables LLM selection; and
- arbitrary free-form LLM selection is disabled, so configured aliases are
  the intended selection mechanism.

Source: [config decoder](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/domain/config.gleam#L1-L32) and [field limits](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/domain/config.gleam#L305-L369).

### Capability strings and local tools

Recognized v0.4.0 capability strings are:

| Capability | Tool exposed to the model | Executed by |
|---|---|---|
| `client_v1_read_file` | `read_file` | CLI |
| `client_v1_edit_file` | `edit_file` | CLI |
| `client_v1_write_file` | `write_file` | CLI |
| `client_v1_execute_shell_command` | `execute_shell_command` | CLI |
| `client_v1_atuin_history` | `atuin_history` | CLI |
| `client_v1_atuin_output` | `atuin_output` | CLI |
| `client_v1_load_skill` | `load_skill` | CLI |
| `client_invocations` | no tool; protocol feature marker | n/a |

Unknown capabilities are ignored. The current client additionally advertises
`client_v2_atuin_history`; core v0.4.0 ignores it, while newer/hosted servers
can use it to select a relaxed history-search schema.

`suggest_command` is a special terminal structured answer rather than an
executed tool. Optional `web_search` and `web_scrape` are executed by the
reference server when configured.

**Important pinned-client caveat:** the inspected client initially routes every
non-`suggest_command` `tool_call` event through its local tool parser. It does
not recognize `web_search`, `web_scrape`, or arbitrary custom server-tool
names. It therefore records a local "unknown tool" error before a later
`tool_result` for that same server call arrives, which can trigger an unwanted
continuation request. The reference server's web-tool event flow is consequently
not transparently compatible with this exact client snapshot. Treat custom
server tools as requiring an end-to-end compatibility test or a client change,
not merely a server registration.

#### Tool-call input schemas

These are the model-facing v0.4.0 schemas. “Required” describes what the
server tells the model to produce; the current client is more lenient for a
few fields, as noted.

| Tool | Required input | Optional input and constraints |
|---|---|---|
| `suggest_command` | `command: string`, `description: string`, `confidence: high/med/low`, `danger: high/med/low` | `confidence_notes: string`, `danger_notes: string` |
| `read_file` | `file_path: string` | `limit: integer` 1–1000 (client default 100 and clamps at 1000), `offset: integer` (the client defaults missing, negative, or otherwise invalid values to 0) |
| `edit_file` | `file_path: string`, `old_string: string`, `new_string: string` | `replace_all: boolean` (default false) |
| `write_file` | `file_path: string`, `content: string` | `overwrite: boolean` (default false; must be true for an existing file) |
| `atuin_history` | `query: string`, `filter_modes: (workspace/directory/session/host/global)[]` | `limit: integer` 1–50 (default 10) |
| `atuin_output` | `history_id: string` | `ranges: [integer, integer][]`, at most 10 ranges. Endpoints are zero-based and inclusive; negative indices count from the end. Core advertises `[[0,1000]]` as the default, but this pinned client treats missing/empty ranges as `[[0,-1]]` and fetches all output. |
| `execute_shell_command` | `command: string`, `shell: bash/fish/zsh/nu`, `description: string` | `timeout: integer` seconds, 1–600 (client default 30 and clamp 600). The wire key is `timeout`, not `timeout_secs`. The client also accepts optional `dir`, though it is absent from the model-facing core schema. |
| `load_skill` | `name: string` | none |

The client parser defaults missing `shell` to `bash` and tolerates missing
shell `description`, despite both being required in the advertised schema.
A replacement should send the advertised shape rather than depending on those
defaults.

Source: [core tool definitions](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/domain/tools.gleam), [client tool parser](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/tools/mod.rs), and [client stream/FSM bridge](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/tui/bridge.rs#L59-L82).

### Success response and SSE framing

Success starts as HTTP 200 chunked SSE:

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
X-Accel-Buffering: no
X-Atuin-AI-Session-Id: <UUID>

```

Each frame is exactly conventional named SSE:

```text
event: <event-name>
data: <single-line JSON>

```

No SSE `id` or `retry` fields are used. The client tolerates unknown event
names and missing/malformed fields quite leniently, but a replacement should
emit the documented forms.

#### Events

**`status`**

```text
event: status
data: {"state":"processing"}
```

Known state values are `processing`, `thinking`, `searching`, and
`waiting_for_tools`. The server emits `processing` first and `thinking` before
each upstream LLM call.

**`text`** — one streamed assistant text delta:

```text
event: text
data: {"content":"partial text"}
```

**`tool_call`** — a completed tool call, not argument deltas:

```text
event: tool_call
data: {"id":"call_123","name":"read_file","input":{"file_path":"README.md"}}
```

**`tool_result`** — emitted only for server-executed tools:

```text
event: tool_result
data: {"tool_use_id":"call_123","content":"...","is_error":false}
```

A persistent hosted implementation may instead emit:

```text
event: tool_result
data: {"tool_use_id":"call_123","remote":true,"content_length":1234,"is_error":false}
```

The standalone reference always uses the inline form because its tool-result
store deliberately declines persistence.

**`done`** — successful terminal event, including a pause for client tools:

```text
event: done
data: {"session_id":"018f7f33-0ad3-7aef-bf0a-d204a18a1234","usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10,"cached_tokens":0,"cache_creation_tokens":0,"input_cost":null,"output_cost":null,"total_cost":null,"provider_cost":null}}
```

The JSON must be on the single `data:` line. The reference encoder does not
emit multi-line SSE data.

The hosted service may add a `credits` object. The standalone server's open
limiter omits it. The current client uses `session_id` and optional `credits`
but ignores `usage`, so an MVP can interoperate with the current client using
`{"session_id":"..."}`. Reproducing the full shape is recommended.

Optional hosted-only addition:

```json
{
  "credits": {
    "period": "calendar_monthly",
    "resets_at": "2026-08-01T00:00:00Z",
    "requests": {"used": 3, "limit": -1},
    "input": {"used": 12345, "limit": 5000000},
    "output": {"used": 678, "limit": 1000000}
  }
}
```

**`error`** — terminal in-stream failure:

```text
event: error
data: {"message":"LLM request failed, please retry","code":"upstream_error"}
```

Known reference codes include `llm_rate_limit`, `llm_unavailable`,
`generation_failed`, `internal_error`, `upstream_error`,
`usage_limit_exceeded`, and (after 50 tool iterations) `generation_failed`.
The current CLI only displays `message`; it ignores `code`.

Source: [SSE encoder and payloads](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/streaming.gleam#L18-L159), [usage payload](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/streaming.gleam#L209-L233), and [client parser](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/stream.rs#L189-L279).

### Typical event sequences

Plain answer:

```text
status(processing) -> status(thinking) -> text* -> done
```

Client-side tool request:

```text
status(processing) -> status(thinking) -> text* -> tool_call+
-> status(waiting_for_tools) -> done
```

The CLI starts permission/execution handling as soon as each `tool_call`
arrives; it does not wait for `waiting_for_tools` or `done`. It opens the next
`POST /api/cli/chat` only after the stream is done and every local tool has
resolved, appending assistant `tool_use` and user `tool_result` blocks.

`suggest_command` terminal answer:

```text
status(processing) -> status(thinking) -> text* -> tool_call(suggest_command)
-> done
```

It has no `tool_result`, `waiting_for_tools`, or follow-up model call. The CLI
returns to idle as soon as it receives `suggest_command` (the later `done` is
therefore not needed for its state transition). In core classification, the
presence of any `suggest_command` takes precedence over accompanying tool
calls; providers should avoid mixing it with other calls because those calls
are emitted but not orchestrated normally.

Server-side web tool (reference engine's intended sequence):

```text
status(processing) -> status(thinking) -> tool_call+
-> status(searching) -> tool_result+ -> status(thinking)
-> (more events) -> done
```

For a response mixing server and client tools (and no `suggest_command`), the
engine executes all server tools first, emits their results in completion
order, then emits the client tool calls followed by `waiting_for_tools` and
`done`. A single HTTP request may therefore contain multiple `thinking` phases
and multiple upstream calls. However, the pinned-client server-tool caveat
above applies to both the server-only and mixed sequences.

### HTTP and error behavior

Application/controller errors before streaming use an HTTP error status and
JSON:

```json
{"error":"invalid_request","message":"..."}
```

Reference cases include:

| HTTP | `error` | Cause |
|---:|---|---|
| 400 | `invalid_request` | Empty/missing messages, invalid message schema, invalid non-empty session UUID, or unresolved configured model |
| 400 | `conversation_too_large` | Estimated conversation exceeds 180,000 tokens |
| 401 | `unauthorized` | Missing/invalid client Bearer token |
| 404 | `not_found` | Unknown route |
| 500 | `internal_error` | Internal pre-stream failure |

The reusable core also supports hosted-only 403 feature-disabled and 429
limit errors, but the standalone server's open limiter does not generate
them. Authentication runs before dispatch, so it can replace an otherwise
expected 404 with 401. Malformed JSON is handled by `Plug.Parsers` outside the
core controller; parser-level failures are not guaranteed to use the JSON
envelope in this table.

Once SSE has started, the HTTP status remains 200. Upstream HTTP errors,
timeouts, malformed provider streams, and generation errors become a terminal
`error` SSE event. Provider status/body detail is logged server-side but is
intentionally replaced with a generic client-facing message.

The current CLI treats any non-2xx chat response as a request error. It has
special messaging for 401. In-stream `error` and `done` terminate its parser.
If the connection simply closes without either, behavior should not be relied
upon; always send a terminal event.

## Upstream OpenAI-compatible protocol

The reference server accepts an operator endpoint with or without a final
`/chat/completions`; it appends that suffix when absent and preserves the
query string. Only `http` and `https` are supported.

It sends:

```http
POST <configured endpoint>/chat/completions
Content-Type: application/json
Authorization: Bearer <upstream key>  # optional/reconfigurable

{
  "stream": true,
  "model": "<resolved provider model ID>",
  "messages": [/* converted OpenAI-format messages */],
  "tools": [/* OpenAI function tools; omitted only if none */],
  "stream_options": {"include_usage": true}
}
```

`stream_options` is merely present in the example operator config, not
hard-coded. Operators can merge arbitrary extra body fields except `stream`,
`model`, `messages`, and `tools`.

The adapter consumes OpenAI-style SSE chat-completion chunks, accumulating
text, reasoning, fragmented tool-call arguments, finish state, and optional
usage. `/v1/responses`-only providers are not supported. Models must support
tool calling for normal agent behavior.

Timing limits in core v0.4.0 are:

- 60 seconds with no LLM chunk or server-tool completion;
- 300 seconds maximum for one upstream LLM stream; and
- 50 tool-loop iterations per client request.

Source: [upstream adapter](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/llm/openai_endpoint.gleam#L1-L223) and [driver limits](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/driver.gleam#L51-L74).

## Where the logic actually lives

### Thin standalone shell

The `atuin-ai-server` repository itself is small. Its Elixir code:

- loads and validates TOML;
- resolves secrets;
- builds model aliases and one upstream option set per model;
- optionally registers Brave search and Firecrawl scrape tools;
- applies static Bearer auth;
- mounts two routes in Bandit/Plug; and
- stores immutable process-wide configuration.

It has no database, accounts, usage limits, transcript recording, or session
storage. Restarting loses no chat state because it did not retain chat state.

### Substantial shared server engine

Most behavior comes from the separately versioned `atuin-ai-core` dependency.
That engine is not a trivial byte-for-byte proxy. It:

- validates and normalizes the Atuin request;
- generates or validates session IDs;
- resolves model aliases;
- builds a large system prompt from platform/shell/cwd/last-command context,
  run preferences, capabilities, contexts, and skills;
- defines JSON Schemas for all model-callable tools;
- converts the client transcript to valid OpenAI messages, including tool
  exchanges and cache-oriented context placement;
- parses incremental provider SSE and fragmented tool arguments;
- runs a state machine for text, client tools, and server tools;
- executes multiple server tools concurrently and may call the model again;
- aggregates token usage across those calls;
- applies a server-side dangerous-command keyword/regex check to
  `suggest_command`, escalating danger to `high` when matched;
- enforces conversation, iteration, and timeout limits; and
- translates provider failures into stable, non-leaking Atuin errors.

Optional web search/scrape is real server behavior, not client behavior.
Scraping is delegated to Firecrawl, so model-selected target URLs are fetched
from Firecrawl's network rather than directly from this server.

### Client responsibilities

The normal Atuin client owns:

- endpoint mode and credentials;
- session/transcript persistence in its local AI database;
- context-window selection and sending the full relevant transcript on every
  request;
- gathering OS, shell, cwd, last-command, user-context, and skill metadata;
- model picker and selected alias persistence;
- the TUI and rendering streaming events;
- local safety/permission prompts, including YOLO mode;
- execution of `read_file`, `edit_file`, `write_file`,
  `execute_shell_command`, `atuin_history`, `atuin_output`, and `load_skill`;
- appending local tool results and making the next chat request; and
- Hub login/usage behavior when the endpoint is classified as Hub.

The key architectural boundary is therefore:

```text
CLI: UI + local state + local tools
        | Atuin JSON/SSE protocol
server: prompting + model/tool orchestration + provider adapter + server tools
        | OpenAI chat-completions JSON/SSE
LLM provider
```

## Drop-in replacement recommendations

### Smallest useful implementation

For an initial implementation compatible with the inspected current CLI:

1. Implement optional static Bearer auth.
2. Implement `/api/cli/models` with at least one model.
3. Accept non-empty `messages`, optional `session_id`, and current `config`
   and `context` fields.
4. Generate a UUID when no session ID is supplied and return it in both the
   response header and `done`.
5. Advertise only capability-authorized client tools to the model.
6. Convert transcripts and tool exchanges correctly to your model API.
7. Stream `status`, `text`, `tool_call`, and terminal `done`/`error` events.
8. For client tools, stop after `tool_call` + `waiting_for_tools` + `done`;
   let the CLI execute and call back.
9. Test both plain text and fragmented streaming tool arguments.

You do not need accounts, a database, credits, `/api/cli/usage`, remote tool
results, Brave, Firecrawl, or the exact reference prompt for basic OSS-client
interoperability.

### If your desired features are fundamentally different

Replacing the server is a good fit when the differences concern:

- model/provider choice;
- system prompts and routing;
- server-side tools or retrieval, provided the pinned client's server-tool
  event-handling limitation is addressed or tested around;
- multi-model orchestration;
- server persistence, policy, quotas, or observability; or
- how tool calls are planned before being handed to the CLI.

It is a less complete fit when the desired difference needs a new local tool,
new TUI interaction, changed permission UI, or data that the current CLI does
not send. The server cannot make an unmodified CLI execute an unknown tool;
the client's tool registry/FSM rejects tools it does not recognize or did not
advertise. Such features require a client change as well.

### Compatibility test matrix

At minimum, run a real Atuin CLI against the replacement and test:

- unauthenticated and authenticated model listing;
- model default and explicit alias selection;
- plain streamed text;
- `suggest_command`;
- every enabled local tool, including a failed tool result;
- multiple tool calls in one model response;
- a server-side tool followed by another LLM call, including the pinned
  client's unknown-local-tool behavior;
- continuation with the same `session_id`;
- malformed requests and invalid session IDs;
- upstream 401/404/429/5xx and unreachable provider;
- provider stream closing early or returning malformed tool JSON;
- client disconnect; and
- old CLI versions you intend to support.

Pin client and server/core revisions in these tests. There is no negotiated
protocol version, and compatibility currently rests on tolerant parsers and
maintaining old wire shapes.

## Operational and security notes

- An unset `AUTH_TOKEN` means open access. Bind only to loopback or put proper
  authentication/TLS in front of it; the server can invoke paid models and
  web services.
- The server accepts model-generated tool calls. Only capability-approved
  local tools should be exposed, and the CLI must remain the enforcement
  point for local permissions.
- If implementing server-side URL tools yourself, add SSRF controls. The
  reference delegates scrape fetching to Firecrawl.
- The reference safety scan for suggested commands is a limited keyword/regex
  backstop, not a shell parser or security boundary.
- Operator-provided custom upstream headers replace default auth headers; they
  do not merge with the default.
- The Docker config defaults to `/etc/atuin-ai/config.toml`; source execution
  defaults to `./config.toml`, overrideable with `CHAT_CONFIG`.
- From a container, an LLM on the host usually needs
  `host.docker.internal`, not `localhost`.
- `atuin-ai-server` and `atuin-ai-core` are Apache-2.0 licensed; the inspected
  Atuin client is MIT licensed. Check the actual license files and preserve
  required notices if copying code rather than independently implementing the
  wire protocol.

## Primary source map

- [Standalone server README](https://github.com/atuinsh/atuin-ai-server/blob/4d582bc5ceea5b5edfdcf3abb49dc850400cda7c/README.md)
- [Standalone router/auth](https://github.com/atuinsh/atuin-ai-server/blob/4d582bc5ceea5b5edfdcf3abb49dc850400cda7c/lib/atuin_ai/server/router.ex)
- [Standalone config/model mapping](https://github.com/atuinsh/atuin-ai-server/blob/4d582bc5ceea5b5edfdcf3abb49dc850400cda7c/lib/atuin_ai/server/config.ex)
- [Standalone engine composition](https://github.com/atuinsh/atuin-ai-server/blob/4d582bc5ceea5b5edfdcf3abb49dc850400cda7c/lib/atuin_ai/server/instance.ex)
- [Core request decoder](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/request.gleam)
- [Core controller](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/controller.gleam)
- [Core SSE protocol](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/http/streaming.gleam)
- [Core agent loop](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/engine/loop.gleam)
- [Core OpenAI adapter](https://github.com/atuinsh/atuin-ai-core/blob/565877e97ed3ff4840481b8d47e12eb2f9854039/src/atuin_ai_core/llm/openai_endpoint.gleam)
- [Client request builder/SSE parser](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/stream.rs)
- [Client transcript encoder](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/tui/state.rs)
- [Client capability list](https://github.com/atuinsh/atuin/blob/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/context.rs)
- [Official AI settings docs](https://docs.atuin.sh/main/ai/settings/)
- [Official self-hosting docs](https://docs.atuin.sh/main/ai/self-hosting/)
