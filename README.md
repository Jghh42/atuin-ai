# atuin-ai

A planned lightweight, self-hosted implementation of the Atuin AI server
protocol, written in Go.

The project is intended to work with an unmodified [Atuin](https://atuin.sh/)
CLI while using OpenAI's public Responses API. Its primary target is a private,
single-user deployment with eligible ChatGPT-plan access through OpenAI's
documented Sign in with ChatGPT OAuth flow.

> **Status:** design and compatibility research. The server has not been
> implemented yet.

## Goals

- Preserve compatibility with the normal Atuin CLI.
- Implement `GET /api/cli/models` and streaming `POST /api/cli/chat`.
- Use the public OpenAI `/v1/responses` API rather than Chat Completions.
- Support secure OAuth credential persistence and refresh for self-hosting.
- Preserve Atuin's local tool and permission boundary.
- Run as a small Go binary with bounded memory and concurrency.
- Maintain a compatibility suite derived from the upstream server, core, and
  client behavior.

## Non-goals for the first version

- A hosted multi-user service.
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

Implementation has not started. The phased work breakdown, proposed package
layout, test strategy, resource limits, and definition of done are in the
[implementation plan](docs/plans/implementation.md).

OAuth credentials, API keys, local configuration, and runtime state must never
be committed to this repository.

## License

A project license has not been selected yet. Upstream material referenced by
the documentation has its own licenses; any copied or transformed material
must retain the required attribution and notices.
