# atuin-ai

A lightweight, self-hosted implementation of the Atuin AI server protocol,
written in Go.

It works with an unmodified [Atuin](https://atuin.sh/) CLI. It runs as a
personal, single-user process on your own machine (including a remote machine
used over SSH) and listens only on `127.0.0.1`. Inference uses your ChatGPT
Business seat through the ChatGPT Codex backend, signed in with a Pi-style
device login. Independently registered Sign in with ChatGPT (SIWC) and public
Responses inference are deferred.

> **Status: experimental v0.1.0.** This is an early single-user 0.x release.
> macOS LaunchAgent setup has been confirmed by a user; **real Linux
> systemd-user setup is still untested**. See the
> [release plan and remaining gates](docs/history/release-v0.1.0.md).

With a valid login, the bundled catalog offers **Sol (`gpt-6.1-sol`, default)**
and **Luna (`gpt-6-luna`)**. Listing models does not contact the upstream
service or prove your account can use them; `auth diagnose` is an explicit
remote check. See the [catalog policy](docs/catalog-policy.md).

## Goals

- Compatibility with the normal Atuin CLI: `GET /api/cli/models` and streaming
  `POST /api/cli/chat`.
- A native Codex Responses adapter, not Chat Completions.
- Isolated OAuth credentials with refresh, via device login that works over SSH
  without a callback tunnel. No Pi/Codex subprocess or credential import.
- IPv4 loopback only. Every request must carry an auto-generated local client
  token, which Atuin sends from `[ai].api_token`
  ([ADR 0006](docs/decisions/0006-required-local-client-token.md)).
- Atuin keeps its local tool and permission boundary.
- A small Go binary with bounded memory and concurrency.
- A compatibility suite derived from the upstream server, core and client.

## Non-goals

- A hosted, multi-user or remote-access service.
- Configurable binding, TLS, containers or Kubernetes.
- Server-side tools, MCP or web tools.
- Atuin Hub billing and usage endpoints.
- Chat Completions, browser cookies, credential scraping, or private endpoints
  beyond the selected Codex adapter.

## Architecture

```text
Atuin CLI
    │ Atuin JSON + SSE (Authorization: Bearer <local client token>)
    ▼
atuin-ai-server (127.0.0.1)
    ├── Atuin protocol decoder and SSE writer
    ├── prompt and capability-gated local-tool policy
    ├── native Codex Responses adapter
    └── OAuth credential manager
             │
             ▼
    https://chatgpt.com/backend-api/codex/responses
```

The Atuin CLI remains responsible for its TUI, conversation persistence,
permissions, and running local file, shell, history, output and skill tools.

## Install

Supported binaries are **Linux amd64 (`GOAMD64=v1`)** and **macOS Apple Silicon
arm64**, both `CGO_ENABLED=0`; no Go SDK is needed. The macOS binary is not
Developer-ID signed or notarized. Use **curl 8.4.0+** and never sudo.

Check the [GitHub Releases page](https://github.com/Jghh42/atuin-ai/releases),
then follow the [pinned download, checksum and installer steps](docs/packaging.md#pinned-installer-after-publication).
The installer puts the verified binary and its legal notices in `~/.local/bin`.
It never logs in, edits configuration or starts a service.

## Usage

```sh
# Attended setup: login, optional user service, optional Atuin config edit.
# Every question defaults to No; an existing login is kept.
atuin-ai-server init

# Or step by step (stop a running server before auth commands):
atuin-ai-server auth login
atuin-ai-server auth status
atuin-ai-server serve            # optional --config PATH

# The local client token Atuin must send as [ai].api_token:
atuin-ai-server token show
atuin-ai-server token rotate     # server must be stopped
```

If you configure Atuin by hand, add:

```toml
[ai]
enabled = true
endpoint = "http://127.0.0.1:17391"
api_token = "<output of atuin-ai-server token show>"
```

The server config accepts only `port` and `default_model`
([example](config.example.toml)). Stop your own server before updating; the
installer does not stop it. See [init](docs/init.md) for services, Atuin config
resolution, backups and removal, and [operations](docs/operations.md) for
configuration, limits, private state and troubleshooting.

## Development

The exact toolchain is **Go 1.27.3**, pinned in `go.mod` and enforced by Make
and CI; normal toolchain auto-download works. See the
[security pin record](docs/provenance/go1273-security.md).

```sh
make check    # format, tests, race, vet, lint, vulnerabilities, fuzz smoke, harness self-tests
make build    # bin/atuin-ai-server
curl -H "Authorization: Bearer $(./bin/atuin-ai-server token show)" \
  http://127.0.0.1:17391/api/cli/models
```

Real-CLI, reference-oracle and resource tests are separate gates that need
pinned builds and Bubblewrap with Linux namespaces (`make compatibility`,
`make resource`). They fail rather than skip without their prerequisites; CI
runs them on every push. See [compatibility](docs/compatibility.md) and the
[resource lifecycle record](docs/provenance/resource-lifecycle.md).

Never commit OAuth credentials, API keys, local configuration or runtime state.

## Documentation

Start at the [documentation index](docs/README.md).

## License

Project-owned code and documentation are licensed under the
[Apache License 2.0](LICENSE) ([ADR 0003](docs/decisions/0003-project-license-proposal.md)).
Third-party material keeps its original licenses and notices; see
[NOTICE](NOTICE) and `LICENSES/`.
