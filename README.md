# atuin-ai

A planned lightweight, self-hosted implementation of the Atuin AI server
protocol, written in Go.

The project is intended to work with an unmodified [Atuin](https://atuin.sh/)
CLI. Its initial target is a personal, single-user process on a developer
machine—including a remote machine used over SSH—that binds only to that
machine's `127.0.0.1`. The selected initial backend is native Pi-style Codex
device login and Responses-format inference through the ChatGPT Codex backend,
targeting the user's ChatGPT Business seat. Independently registered Sign in
with ChatGPT (SIWC) and public Responses inference are deferred for later.

> **Status:** phases 0–3 implemented: isolated Codex proof, bounded HTTP shell,
> Atuin decoding, and unit-tested prompts/tools/safety. Phase 3a adds bounded
> native model discovery/cache behind injected credentials and port 17391.
> Live discovery returned no replay-proven model: its eligible-catalog gate is
> **NOT PASSED**. Production login wiring remains Phase 7; standalone models/chat
> return 503; injected runtime dependencies enable chat generation and SSE.
> Phase 4 adds tested native Responses request projection/HTTP opening behind
> injected credentials. Phase 5 adds the bounded native SSE provider with
> explicit completion, buffered calls, token usage and cancellation. Independent
> review cleared that checkpoint. Phase 6 now integrates the one-generation turn
> coordinator, atomic CLI tool gate and bounded SSE writer behind injection;
> independent final review cleared the checkpoint. Production login is not added.
> See [Phase 6](docs/plans/phase6-status.md),
> [Phase 3a](docs/plans/phase3a-status.md),
> [Phase 4](docs/plans/phase4-status.md), and
> [Phase 5](docs/plans/phase5-status.md) checkpoints.

## Goals

- Preserve compatibility with the normal Atuin CLI.
- Implement `GET /api/cli/models` and streaming `POST /api/cli/chat`.
- Use a native Codex Responses-compatible adapter, not Chat Completions.
- Support secure, isolated OAuth credentials and refresh through Pi-style Codex
  device login; no Pi/Codex subprocess or credential-file import.
- Support headless/SSH development with browser device approval and no callback
  tunnel. Add independently registered SIWC/public Responses as a later feature.
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
- Chat Completions, browser cookies, credential scraping, or private endpoints
  outside the explicitly selected Codex compatibility adapter.

## Documentation

- [Atuin AI protocol specification](docs/specs/atuin-ai-server-api.md)
- [Go rewrite implementation plan](docs/plans/implementation.md)
- [Architecture decision records](docs/decisions/README.md)
- [Authentication prerequisites and live proof checklist](docs/authentication.md)
- [Compatibility fixture inventory](docs/compatibility.md)
- [Third-party provenance and pinned sources](docs/provenance/README.md)

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
    ├── native Codex Responses-compatible adapter
    └── OAuth credential manager
             │
             ▼
    https://chatgpt.com/backend-api/codex/responses
```

The Atuin CLI remains responsible for its TUI, conversation persistence,
permissions, and execution of local file, shell, history, output, and skill
tools.

## Development

```sh
make check
make build
./bin/atuin-ai-server serve # optional --config config.example.toml
curl http://127.0.0.1:17391/api/cli/models # 503 until production login is wired
```

See [local operations](docs/operations.md) for configuration precedence,
limits, shutdown, and the deliberately unwired standalone credential lifecycle. The
remaining phased work is in the [implementation plan](docs/plans/implementation.md).

OAuth credentials, API keys, local configuration, and runtime state must never
be committed to this repository.

## License

Project-owned code and documentation are licensed under the
[Apache License 2.0](LICENSE), as recorded in [ADR 0003](docs/decisions/0003-project-license-proposal.md).
Third-party material retains its original licenses and notices; see
[NOTICE](NOTICE) and `LICENSES/`.
