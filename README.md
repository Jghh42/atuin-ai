# atuin-ai

A planned lightweight, self-hosted implementation of the Atuin AI server
protocol, written in Go.

The project is intended to work with an unmodified [Atuin](https://atuin.sh/)
CLI while using OpenAI's public Responses API. Its initial target is a
personal, single-user process that binds only to `127.0.0.1`, with eligible
ChatGPT Business plan access through OpenAI's documented Sign in with ChatGPT
OAuth flow.

> **Status:** phase 1 HTTP shell implemented; chat returns HTTP 501. Phase 0
> live SIWC proof and project-license confirmation remain review gates. No
> inference or OAuth login implementation is shipped yet.

## Goals

- Preserve compatibility with the normal Atuin CLI.
- Implement `GET /api/cli/models` and streaming `POST /api/cli/chat`.
- Use the public OpenAI `/v1/responses` API rather than Chat Completions.
- Support secure local OAuth credential persistence and refresh.
- Listen only on IPv4 loopback in the first version, with no client-facing
  authentication or remote-host deployment surface.
- Preserve Atuin's local tool and permission boundary.
- Run as a small Go binary with bounded memory and concurrency.
- Maintain a compatibility suite derived from the upstream server, core, and
  client behavior.

## Non-goals for the first version

- A hosted multi-user or remote-access service.
- Configurable/non-loopback binding, client Bearer auth, TLS, containers, or
  Kubernetes deployment.
- Arbitrary server-side tools, MCP, or web tools.
- Atuin Hub billing and usage endpoints.
- Chat Completions or undocumented ChatGPT backend APIs.

## Documentation

- [Atuin AI protocol specification](docs/specs/atuin-ai-server-api.md)
- [Go rewrite implementation plan](docs/plans/implementation.md)
- [Architecture decision records](docs/decisions/README.md)

The protocol specification is based on pinned revisions of:

- `atuinsh/atuin-ai-server`
- `atuinsh/atuin-ai-core`
- `atuinsh/atuin`

See the specification for exact commit IDs, wire formats, authentication
behavior, endpoint schemas, and source references.

## Proposed architecture

```text
Atuin CLI
    │ Atuin JSON + SSE
    ▼
Go compatibility server
    ├── Atuin protocol decoder and SSE writer
    ├── prompt and capability-gated local-tool policy
    ├── OpenAI Responses adapter
    └── OAuth credential manager
             │
             ▼
    https://api.openai.com/v1/responses
```

The Atuin CLI remains responsible for its TUI, conversation persistence,
permissions, and execution of local file, shell, history, output, and skill
tools.

## Development

```sh
make check
make build
./bin/atuin-ai-server serve --config config.example.toml
curl http://127.0.0.1:8080/api/cli/models
```

See [local operations](docs/operations.md) for configuration precedence,
limits, shutdown, and the intentionally unimplemented chat endpoint. The
remaining phased work is in the [implementation plan](docs/plans/implementation.md).

OAuth credentials, API keys, local configuration, and runtime state must never
be committed to this repository.

## License

A project license has not been selected yet. Upstream material referenced by
the documentation has its own licenses; any copied or transformed material
must retain the required attribution and notices.
