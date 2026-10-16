# Pi Codex compatibility probe provenance

The initial native Go probe is source-informed by the installed
`@earendil-works/pi-ai` **1.1.0**, published from
https://github.com/earendil-works/pi (`packages/ai`). No Pi/Node/Codex runtime is
linked or spawned. No user credential files were read. No upstream test
fixtures were imported; the fake-server/security tests are independently
written for this probe.

Protocol reference artifacts (SHA-256, installed package):

| Artifact | SHA-256 |
| --- | --- |
| `dist/auth/oauth/openai-codex.js` | `0b94a219ddb5982f0ee543ef03138ed0254ce6ee0a57c9d89d39c242e2eaf6f6` |
| `dist/auth/oauth/device-code.js` | `8f197cc9af67be64b82939573d719d1b55388f96f2e53cb8c47f9618fc1297eb` |
| `dist/api/openai-codex-responses.js` | `b185956e11a5c5cb522e5cad77daf21ce31c3c304e86a8333ffcb49ff78466ea` |

The public npm registry metadata at
https://registry.npmjs.org/@earendil-works/pi-ai/1.1.0 identifies the package
license as MIT and records tarball integrity:
`sha512-1T7LAkc/5Bvc0v6w4vAGVdCrli0o/E0pEmYKTnixu95vSFArBjvbhS/G4ZwI0RUePgf0Imcu0VyqlM4EcXxqfw==`.
The package/artifact hashes, not mutable GitHub `main`, pin the inspected source.
An exact matching upstream Git commit has not been established; do not invent
one from artifact filenames.

`internal/codexproof/auth.go` is a Go adaptation of the referenced device-login,
polling and token-exchange protocol. It differs by using bounded HTTP responses,
strict safe errors, no retries, explicit cancellation, an isolated lifetime-
locked atomic credential store, and private browser code display rather than
logging. `internal/codexproof/inference.go` is independently implemented using
our bounded proof-parser pattern, with Pi's Codex endpoint, headers, request
fields and function-call protocol as source references. It deliberately tests
full semantic history without opaque-state retention instead of Pi's broader
state/transport handling. It does not implement Pi's agent/tools or import its
credentials.

Retain MIT attribution: **Copyright (c) 2025 Mario Zechner**. The unmodified
upstream MIT text fetched from
https://raw.githubusercontent.com/earendil-works/pi/main/LICENSE is retained at
[`LICENSES/Pi-MIT.txt`](../../LICENSES/Pi-MIT.txt). The fetched license URL is
mutable; npm independently declares MIT for the pinned package. Project-owned
code remains Apache-2.0; retain applicable MIT notices with source/binaries.
This provenance record is not a support guarantee for the Codex registration
or backend and is not successful live validation evidence.
