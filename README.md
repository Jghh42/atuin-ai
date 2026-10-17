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

> **Development candidate:** native login/refresh/logout and bounded Responses
> streaming are implemented. With valid application credentials, the bundled
> catalog offers **Sol (`gpt-6.1-sol`, default)** and **Luna (`gpt-6-luna`)**.
> Listing does not fetch remote metadata or establish account entitlement.
> `auth diagnose` remains an explicit remote check, not an availability gate.
> Luna live validation is pending. Both-model native proof, current Mac and
> production live lifecycle gates still block release. No release is published.
> [Authorized policy and proof boundaries](docs/plans/bundled-catalog-policy.md).
> [Historical progress / CI receipts](docs/plans/autonomous-progress.md).

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

The project minimum and exact Make/CI toolchain pin is **Go 1.27.2**. Normal
Go toolchain auto-download is supported. This current security-patched release
also fixes the fuzz coordinator's budget-boundary race; all fuzz budgets and
failure assertions remain unchanged. [Diagnosis and pin rationale](docs/plans/fuzz-toolchain-diagnosis.md).

```sh
make check
make build
# Interactive, explicit opt-in login; do not run live unattended:
./bin/atuin-ai-server auth login
./bin/atuin-ai-server auth status # stop serve before separate auth commands
./bin/atuin-ai-server serve # optional --config config.example.toml
curl http://127.0.0.1:17391/api/cli/models # valid login required; no metadata GET
```

See [local operations](docs/operations.md) for configuration precedence,
limits, shutdown, private state and the single-process credential lifecycle.
[Authentication](docs/authentication.md) records the security boundary and
unpassed, opt-in live validation steps. The
remaining phased work is in the [implementation plan](docs/plans/implementation.md).

Real-CLI and differential tests are separate local artifact-gated checks:
default checks explicitly skip them without fresh build manifests; a skip is
**not** a gate pass. `make compatibility` fails without prerequisites, including
Bubblewrap 0.11.1 with usable Linux user/PID/mount namespaces. Secret-free
PR/push/manual CI runs actual compatibility and exposes a failure-propagating
`Phase8 validation aggregate` check (admin branch rules must require it).
CI now builds checksum-pinned Bubblewrap into an owned prefix and probes the
actual namespace operations. The first hosted run demonstrated AppArmor/namespace
setup and actual CLI success, not overall compatibility success; differential and
aggregate failed. No developer-host policy change is performed. See the [Phase 8 commands,
source/toolchain pins and isolated fake-only setup](docs/plans/phase8-status.md).
No production/live or resource gate is cleared by these synthetic tests.

OAuth credentials, API keys, local configuration, and runtime state must never
be committed to this repository.

## License

Project-owned code and documentation are licensed under the
[Apache License 2.0](LICENSE), as recorded in [ADR 0003](docs/decisions/0003-project-license-proposal.md).
Third-party material retains its original licenses and notices; see
[NOTICE](NOTICE) and `LICENSES/`.
