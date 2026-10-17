# atuin-ai

A lightweight, self-hosted implementation of the Atuin AI server protocol,
written in Go.

The project is intended to work with an unmodified [Atuin](https://atuin.sh/)
CLI. Its initial target is a personal, single-user process on a developer
machine—including a remote machine used over SSH—that binds only to that
machine's `127.0.0.1`. The selected initial backend is native Pi-style Codex
device login and Responses-format inference through the ChatGPT Codex backend,
targeting the user's ChatGPT Business seat. Independently registered Sign in
with ChatGPT (SIWC) and public Responses inference are deferred for later.

> **Initial experimental release planned: v0.1.0.** Publication is authorized,
> but no release asset is claimed available by this preparation checkpoint.
> Native macOS LaunchAgent/init and Atuin's test reaching ready were
> user-confirmed on `e27e189`; **real Linux systemd-user setup is still untested**.
> This is an early single-user 0.x release, not a production-maturity claim.
> [Release plan, evidence and remaining publication gates](docs/plans/release-v0.1.0.md).
>
> With valid application credentials, the bundled catalog offers
> **Sol (`gpt-6.1-sol`, default)** and **Luna (`gpt-6-luna`)**. Listing does not
> fetch remote metadata or establish account entitlement. `auth diagnose` is an
> explicit remote check, not an availability gate. Model/provider availability
> can vary; earlier live/probe results are not blanket lifecycle guarantees.
> [Catalog policy and historical proof boundaries](docs/plans/bundled-catalog-policy.md).

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

## Install (after v0.1.0 publication)

Check the [GitHub Releases page](https://github.com/Jghh42/atuin-ai/releases)
for the complete published release first; the planned downloads are not yet a
post-publication test receipt. Supported binaries are **Linux amd64
(`GOAMD64=v1`)** and **macOS Apple Silicon arm64**, both `CGO_ENABLED=0`;
installing them needs no Go SDK. Intel macOS and Linux arm64 are not included.
The macOS binary is not Developer-ID signed/notarized; no Gatekeeper bypass is
provided. Use **curl 8.4.0+**, including for the outer bootstrap download, and
never sudo.

Follow the [pinned HTTPS download, checksum and installer steps](docs/packaging.md#pinned-installer-after-publication).
They use `curl -q` first, verify the downloaded installer, then install a verified
raw binary **and its versioned legal notices** beside it in `~/.local/bin`.
The installer never logs in, edits config/profiles or starts a service. Afterward:

```sh
"$HOME/.local/bin/atuin-ai-server" version
# Optional attended wizard: reuse/login, optional user service, optional Atuin config.
# Every consent defaults to No; existing application login is retained.
"$HOME/.local/bin/atuin-ai-server" init
# Or choose foreground operation after this application's own login:
"$HOME/.local/bin/atuin-ai-server" serve
```

Stop your own running server before an update or separate auth operation;
installation does not stop it for you. Init can offer a temporary stop only for a
verified-owned managed service. See [init safety and service instructions](docs/init.md).
The server still accepts only `port` and optional `default_model`; listening is
IPv4 loopback only (`127.0.0.1:17391` by default), not a remote/multi-user service.

## Development

The project minimum and exact Make/CI toolchain pin is **Go 1.27.2**. Normal
Go toolchain auto-download is supported. This security-patched Go toolchain
also fixes the fuzz coordinator's budget-boundary race; all fuzz budgets and
failure assertions remain unchanged. [Diagnosis and pin rationale](docs/plans/fuzz-toolchain-diagnosis.md).

```sh
make check
make build
# Interactive setup: retain/login, optional user service, optional Atuin config.
# Every consent defaults to No. No controlling terminal => no changes.
./bin/atuin-ai-server init
# Or use separate commands (stop server first); no unattended live login:
./bin/atuin-ai-server auth login
./bin/atuin-ai-server auth status # stop serve before separate auth commands
./bin/atuin-ai-server serve # optional --config config.example.toml
curl http://127.0.0.1:17391/api/cli/models # valid login required; no metadata GET
```

See [native init setup](docs/init.md) for optional macOS/Linux user services,
conservative Atuin18.19/newer config resolution, backups and removal. The bootstrap
installer remains noninteractive and never invokes init automatically.

See [local operations](docs/operations.md) for configuration precedence,
limits, shutdown, private state and the single-process credential lifecycle.
[Authentication](docs/authentication.md) records the security boundary and
unpassed, opt-in live validation steps. The
remaining phased work is in the [implementation plan](docs/plans/implementation.md).

Real-CLI and differential tests are separate local artifact-gated checks:
default checks explicitly skip them without fresh build manifests; a skip is
**not** a gate pass. `make compatibility` fails without prerequisites, including
Bubblewrap 0.11.1 with usable Linux user/PID/mount namespaces. Secret-free
PR/push/manual CI runs the pinned 20 CLI/14 oracle cases and exposes a
failure-propagating `Phase8 validation aggregate` check. Resource execution has
its own cleanup/count aggregate; Go checks include native synthetic macOS TTY
regressions. [Current baseline and release gate distinctions](docs/plans/release-v0.1.0.md).
CI builds checksum-pinned Bubblewrap into an owned prefix and probes actual
namespace operations. No developer-host policy change is performed. See the
[Phase 8 commands, pins and historical failures](docs/plans/phase8-status.md).
Synthetic compatibility/resource success is not live provider or native Linux
user-service validation.

OAuth credentials, API keys, local configuration, and runtime state must never
be committed to this repository.

## License

Project-owned code and documentation are licensed under the
[Apache License 2.0](LICENSE), as recorded in [ADR 0003](docs/decisions/0003-project-license-proposal.md).
Third-party material retains its original licenses and notices; see
[NOTICE](NOTICE) and `LICENSES/`.
