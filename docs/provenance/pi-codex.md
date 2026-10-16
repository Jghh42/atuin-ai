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
