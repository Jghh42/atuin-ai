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
| `dist/api/openai-responses-shared.js` | `8744ad2ce9ce2512993360600760f5bcdbeb520d0a641c655b2a0b7cbe9db10a` |
| `dist/api/transform-messages.js` | `9d747a3d64c533f7bfaf2a66e8446dc006086559d364a09d4c51d1c8c9c332e5` |

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

## Phase 5 native stream parsing (2026-10-16)

`internal/provider/codex/{stream,sse,parser,json}.go` and tests are independently
written Go, using the pinned shared adapter's `processResponsesStream`
(lines 321–699) as a protocol reference (read during implementation). No code
or fixture text copied. Unlike Pi, all refusal/reasoning and unsupported output
channels fail closed; explicit completed status is mandatory, argument/text
snapshots must reconcile, and token numbers and all buffers are bounded. No
opaque state, tool execution, pricing, incomplete-success mapping or transport
retry/fallback is adapted. Tests/fixtures are safe synthetic data described in
`testdata/responses/README.md`; precise policy is in the Phase 5 checkpoint.
Existing MIT attribution/license above applies to this source-informed work.
Independent-review fixes add strict duplicate-member/Unicode/depth/work checks,
response-identity tracking, sparse-index reconciliation and unsupported execution
channel rejection. Nonempty nonterminal lifecycle output is explicitly rejected,
not silently discarded or partially reconciled; absent/empty arrays remain
supported, while explicit null/non-array output fails. `internal/jsoncheck/unicode.go` is extracted from this
project's pre-existing tools validator (not from Pi); both actual callers share
its unchanged Unicode-checking behavior. New review regressions are original
synthetic data, not captured traffic or adapted upstream fixtures.

## Phase 4 production request projection (2026-10-16)

`internal/provider/codex/request.go` is independently written Go using the same
pinned adapter's `buildRequestBody` (lines 373–430), `buildSSEHeaders`
(lines 1281–1308), and the native proof's synthetic text/tool replay as protocol
references. The installed adapter hash was reverified exactly before coding.
Its imported `dist/api/openai-responses-shared.js` was also inspected (message,
function call/output and tool converters, lines 130–310); SHA-256:
`8744ad2ce9ce2512993360600760f5bcdbeb520d0a641c655b2a0b7cbe9db10a`.
This is the same installed Pi AI 1.1.0 package/license above. The shared
converter imports `./transform-messages.js` and invokes `transformMessages`
before conversion (line 90). The dependency's full installed source was read
and its SHA-256 verified during independent-review fixes; exact package-relative
path/hash are in the table above. Its second pass inserts synthetic error results
with `No result provided` for outstanding calls before the next assistant/user
turn or at history end. This materially affects wire shape: the shared
converter alone is not the complete Pi history transformation.

Verified body details: flat function declarations; Codex passes `strict:null`
to the shared converter (not strict:true), baseline `text.verbosity:low`,
`tool_choice:auto`, `parallel_tool_calls:true`, and encrypted reasoning include.
The native proof demonstrates string-content user/assistant message shorthand,
function calls without item IDs, and output strings. We use that proven text
shape rather than adapting Pi's synthetic assistant item IDs/annotations.
The proof's strict:true empty synthetic tool is not used as a production schema
policy: actual pinned Codex declarations use null strictness. Optional Pi
reasoning/model flags, grammar tools, additional tools, cache keys, WebSockets,
continuation optimization, retries and transport fallback are not adopted.
Pi's synthetic missing-result repair is also deliberately **not** adopted.
The native proof only demonstrates paired function calls/results, not a
result-less terminal Atuin suggestion. Ordinary unresolved local calls remain
invalid. As an explicit coordinator-approved Atuin compatibility exception,
historical `suggest_command` becomes assistant text with the fixed original
prefix `Terminal command suggestion (suggest_command; suggestion only, not evidence of execution). Original arguments JSON: `
followed by its unchanged raw argument JSON, preserving whitespace and numeric
lexemes. This avoids both dangling Responses calls and invented execution
outputs. It is a deliberate new projection policy, not a copied Pi repair or
live-validated claim. Synthetic HTTP regressions cover history ending at the
suggestion and continuation after a new user turn alongside paired local tools.

The separate first-input turn context, `Tool execution failed: ` error prefix,
new provider interfaces, bounded request checks, safe errors and all synthetic
Go test fixtures are original project design/text. Existing prompt/schema
provenance is unchanged. No upstream fixture/code text was copied, and no Pi
runtime, store, or subprocess is used. Retain the MIT attribution above with
this source-informed request implementation as well as the initial proof.

## Phase 7 production native device lifecycle (2026-10-16)

`internal/credentials/oauth.go` is a new Go protocol adaptation, not an import of
Pi or the isolated proof package. Before implementation, the installed package
metadata was read (name/version **@earendil-works/pi-ai 1.1.0**, MIT), both full
`dist/auth/oauth/openai-codex.js` and `dist/auth/oauth/device-code.js` artifacts
were read, and their SHA-256 hashes reverified exactly against the table above:
`0b94a219ddb5982f0ee543ef03138ed0254ce6ee0a57c9d89d39c242e2eaf6f6`
and `8f197cc9af67be64b82939573d719d1b55388f96f2e53cb8c47f9618fc1297eb`.
No exact Git commit is asserted. The isolated proof source and fake tests were
read only as references, never used as runtime or credential input.

Reviewed facts: fixed registration, usercode/poll/token endpoints, fixed device
activation/redirect, returned code+PKCE verifier form exchange, access-token
account claim, rotating refresh form and 15-minute flow timeout. No Pi browser
OAuth/callback code, scopes, DCR, SIWC namespace/OIDC rules or credential store
is imported. Claim decoding is explicitly NOT signature verification.

Differences: terminal-only code display; fixed URLs with injected transport;
20-second HTTP/body/header bounds and no redirects; HTTP/1-only native transport
without connection pooling to avoid net/http raw unsolicited-idle-response logs;
duplicate-name equivalence follows Go Unicode SimpleFold (including non-ASCII
aliases) rather than lowercasing, while persisted names remain exact; strict Unicode/
depth JSON rejection; default five-second/minimum one-second interval, 5-second
slow_down increments and bounded expiry; explicit OAuth errors take precedence
over 403/404 and even success (unlike Pi's status-first pending branch); malformed
nonempty pending bodies fail closed; strict PKCE verifier and account-header
shape; earlier claim/response expiry. Tests are independently authored synthetic
Go data, not upstream fixtures or captured traffic.

`manager.go`, `store_unix.go`, application composition and redaction/binding
helpers are original project code. The durable secret-free pre-refresh intent,
fail-closed ambiguous rotation/restart strategy, independent auth_epoch versus
generation, descriptor-relative store and cancellation-independent replacement
save are deliberate hardening beyond the isolated proof/Pi reference. No runtime
Pi/Codex subprocess, app-server, harness, automatic import, browser interaction
or live call was used. Existing MIT attribution/license applies to the protocol
adaptation; the new Unix dependency is recorded in [dependencies](dependencies.md).

## Bundled catalog reference (2026-10-17)

The explicitly user-approved [policy correction](../plans/bundled-catalog-policy.md)
uses only public installed package data/code, not user profile/settings/auth/models
files. Both installed packages identify version **1.1.0**. Public artifacts were
read and hashed; paths below are relative to their package roots:

| Package / artifact | SHA-256 |
| --- | --- |
| pi-ai `dist/providers/data/openai-codex.json` | `3cc35a7e122e5c77ee8ec30a9a3bd8bd59629997e85ba2cb3f9e0ed62e9a09c0` |
| pi-ai `dist/providers/openai-codex.models.js` | `6cf3121e84d836dccf2c738c205884dbecce844b80a168035de4adb9ba218343` |
| pi-ai `dist/providers/openai-codex.js` | `44087142be22adfd09cf1a87edbbdb74ce155b5fd34e70fa4666f3a9f8a9a680` |
| pi-coding-agent `dist/core/remote-catalog-provider.js` | `1adadd993f0c2efe8326480f5ac8ba3d3300a02ef1089f022907b1a5e4385499` |

The built-in data literally contains `gpt-6.1-sol` and `gpt-6-luna` under
`openai-codex-responses`, both `contextWindow: 272000`, `maxTokens: 128000`.
The generated models module loads that JSON and the provider supplies those
built-in models. The coding-agent overlay merges dynamic metadata with a static
baseline; its pi.dev refresh/retry/persistence implementation is NOT ported.
This patch implements only the two-ID local baseline, no automatic enrichment.
No prices, images, reasoning flags or capacity guarantees are adopted. The
existing 180000 estimated-input cap remains, not a promise of 180k input plus
128k output simultaneously. Names/descriptions and all Go code/tests are original.
Existing MIT attribution (Copyright (c) 2025 Mario Zechner) and retained license
above apply to this source-informed metadata work; no third-party implementation
text was copied. This is not live proof, entitlement or an upstream support claim.

Separately, the pinned public Codex checkout `96eb400d1231f0199e96f640c740096087cf3e9b`
contains literal `gpt-6.1-sol` in `codex-rs/models-manager/models.json` (SHA-256
`943ca7fe1d19ed019054158303f0dbbd80b3bef43b3aa425a71aa7cb3fb52c2b`).
It corroborates the exact Sol slug, not Luna live acceptance. No probe credentials,
private source or user state were consulted. Both-model native/current Mac/live
release gates remain separate from this reference evidence.
