# Phase 8 — pinned real CLI and differential compatibility

2026-10-17 delivery checkpoint: **ALL final independent reviews CLEARED**;
current-artifact local compatibility and independent race verification PASSED.
Normal direct-main delivery is authorized. Exact-HEAD ALL-workflow CI (including
actual source-prefix namespace setup and compatibility) is a separate gate and
was not run at preparation time. Historical snapshots/failures below are retained,
not current pending-review claims or retrospective isolation proof.
Started from clean main
`b990c404e4582246a8c2ff0a067b33f14b148ca0` (Phase 7 delivery).
Coordinator-verified exact-HEAD Go checks run `38974738689`: **success**.
This previous-SHA evidence is not CI for the Phase 8 delivery SHA.

## Scope / retained blockers

Only Phase 8 test infrastructure, synthetic fixtures, provenance and docs were
added. **No production/runtime/auth/catalog/tool policy was changed.** No
protocol bug requiring a runtime fix was found. Existing strict validation and
atomic tool buffering are preserved. The server remains native Go, without a
Pi/Codex/app-server/actor/harness inference subprocess or new runtime knobs.
The compiled unmodified Atuin owns local execution, permissions and persistence.

**Fresh production login/live lifecycle and live eligible discovery remain NOT
PASSED.** Historical isolated Phase 0 and retained-probe generation 3 evidence
remain separate. No real account/key, live login or probe credential import was
intentionally supplied or invoked by the harness. However, initial HOME/XDG-only
isolation did NOT confine ancestor context discovery/interpolation. Those old
runs cannot establish absence of external context reads/command execution. No
incident is inferred and no private ancestor contents were inspected. Corrected
runs now enforce the filesystem/PID namespace boundary described below. `gpt-6.1-sol` appears only as the existing eligible
ID in synthetic discovery, NOT evidence of live availability or new eligibility.
Phase 9 packaging/resource gates are untouched; release remains blocked.

## Real client

Built Atuin `4ca8b77c8a526b8c294eb864a2b74c95c685c983` with official pinned Rust
1.98.0, locked dependencies and AI/default features. Full tracked source trees,
including sparse-excluded vendor files, are now freshly archived without
patches into a new private run directory, with COMPLETE inventory (extra files,
directories, modes and symlinks) checked before/after compilation. Toolchain
components are freshly installed from checksum-verified official archives;
Cargo HOME/target/config ancestors are fresh and namespace-isolated. Original
HEADs and clean worktrees were rechecked. Latest fresh cold build took 11m06s
within the 30-minute budget; earlier 10m53s corrected and 11m05s cached-input
builds are retained as historical, not current recipe evidence. [Exact hashes, flags, licenses, image/tool
pins and resource observations](../provenance/phase8-tooling.md).

`tests/e2e/cli_test.go` starts the real native `application.Runtime` and
`httpserver.Server` on IPv4 loopback. Injected transport synthesizes only the
fixed device/token/discovery/Responses destinations, rejects every other
destination, checks exact fake token/account binding and records generation
requests. A loopback-only test proxy records actual CLI requests/session
headers and provides a test-only completion barrier. It is not in the runtime.

`confinement.py` FIRST creates a Bubblewrap filesystem/user/PID namespace,
with an fd-pinned private `/fixture`, read-only pinned executable/system `/usr`,
new `/proc` and `/dev`, and synthetic public `/etc` data. Host HOME, /tmp,
ancestors, projects, skills and private stores are not mounted. The virtual
root is read-only; fixture symlinks/unapproved context/project metadata fail
closed before EACH child launch. Root FD binding survives adversarial ancestor
path replacement. This is not a world-writable-/tmp once-check assumption.

Inside that boundary, Python stdlib PTY automation runs the actual
`atuin ai inline --hook`, not a replacement client. Every invocation (including preparation/status/history/output/daemon/
PTY-proxy processes) uses an allowlisted environment with dedicated HOME,
ATUIN_HOME, all XDG paths, TMPDIR, databases and sandbox. Sync/update/logging
are disabled, OSS is explicit, no client token is configured, and non-loopback
HTTP proxies point to a dead local port. Daemon socket/pidfile are explicitly
sandboxed, periodic sync/autostart/systemd activation disabled, and ready daemon
PID/socket ownership is asserted. No shell profile or real skill/store is inherited. Terminal geometry is set before exec; keys and shell-hook markers
come from pinned source. No screenshots or raw credential contents are saved.

Twenty real-client subcases cover:

- model list/default and real `/model` picker selection of the eligible fake
  model; complete text history, server-issued session reuse and one generation
  per actual Atuin request;
- two separate CLI processes resuming persisted synthetic text history;
- terminal `suggest_command`: Tab returns the exact **insert**, never execute,
  shell-hook outcome;
- all seven local tools: actual file read/write, read-before-edit, harmless
  Bash output, seeded history search (returned unique history ID, not just a
  query echo), skill registry load, and captured-output
  retrieval through an actual isolated daemon;
- output fixture: actual `history start/end` supplies the history ID; a
  source-verified synthetic OSC marker fixture feeds the actual PTY proxy and
  capture sink. Actual daemon output search acknowledges storage before the
  model asks `atuin_output`. This proves retrieval/correlation, not a complete
  shell-hook installation or arbitrary user-shell capture rollout;
- Deny permission with no file write, shell exit 7, parallel calls with distinct
  targets and exactly one correlated historical call/result per ID;
- an announced executable call parsed before the visible text witness and held
  before `response.completed`: the downstream proxy asserts NO executable
  `tool_call` frame before release; a fresh permission witness must follow
  release, then actual CLI write and continuation. File absence alone is NOT
  treated as a buffering proof;
- failed provider after visible text with an announced call: client error,
  no file, no retry; mixed suggestion/local batch likewise never executes;
- real CLI disconnect cancelling fake upstream context promptly;
- missing credentials (no generation), expired credentials (exactly one fake
  refresh), revoked `invalid_grant` (no generation), and explicit synthetic
  reauthentication restoring a new CLI invocation. These are NOT live OAuth.

The model/picker case includes a synthetic writable HOST ancestor project,
`TERMINAL.md` / `.atuin/TERMINAL.md` with an embedded command, and parent skill.
None may appear in requests or execute. Separate regressions cover symlinks,
fd-pinned ancestor replacement and namespace-root invisibility using only
synthetic contents, never private ancestor reads.

Both fake backends now validate outbound model, stream/stateless flags, exact
ordered enabled tool wrappers/schemas (only description annotations differ),
leading context, and COMPLETE ordered text/call/result history before canned
success. `tests/projection` is independent of production decoder/projector/tool
registry; mutations dropping/reordering tools/history/IDs/constraints must fail.

CLI request and upstream generation counts must match exactly. Continuations
must reuse the HTTP session ID, include complete history and correlate
Responses `call_id` (not output-item ID). Fake secrets/provider-private errors
are forbidden from client output. All normal exits assert actual hook markers.

## Differential oracle

Original server `4d582bc5ceea5b5edfdcf3abb49dc850400cda7c` and locked core
`565877e97ed3ff4840481b8d47e12eb2f9854039` were actually built and run. The
oracle Dockerfile changes build tooling only, checks server source hashes and
core clean HEAD, pins official base digests/Gleam checksum and uses a signed
historical Debian snapshot after mutable apt pool 404s. No reference app source
is patched. The pre-existing latest image is NOT accepted as pinned evidence.

The oracle and deterministic fake Chat Completions service run on an internal
Docker bridge with **no published ports** or real keys. A restricted test-only
HTTP helper inside the network sends identical JSON fixture bytes; native Go
receives them over loopback with semantically equivalent fake Responses. This
is a container test oracle, not a deployment/container feature.

Fourteen fixtures cover text, suggestions, all seven tool declarations, parallel
calls, completed-history continuation and three asserted intentional policies.
UUID shape, header/done identity and explicit requested continuation identity
are validated BEFORE normalization; malformed/missing/changing IDs cannot be
normalized into validity. All raw token/cost fields are checked first. Positive
events then compare after ONLY valid session UUID, usage, property-order and
adjacent text-segmentation normalization. Tool IDs, input,
risk, lifecycle events, errors and ordering are not erased. Both backends must
generate exactly once per fixture; CC received/validated counts must equal the
case count and unknown/invalid counts must be zero. Model list/default JSON is
also compared. Intentional-policy cases assert COMPLETE expected error/call/
status/done payloads; enhanced risk reconciles only its asserted risk/notes
change and then compares every other event and payload unchanged.

Intentional differences are explicit tests, not normalization rules:

1. Native rejects mixed suggestion/local batches atomically; reference emits
   both calls and completes.
2. Native raises `sudo true` from low to high with the exact privileged warning;
   reference retains low. Neither oracle executes a suggestion.
3. Missing upstream token usage remains native JSON null (and costs unknown),
   while reference token counts default to zero and absent costs serialize as
   the **string `"nil"`**, not JSON null. The usage case asserts all five raw
   token fields and all four costs BEFORE permitted normalization; remaining
   events must still compare. This reference codec quirk is not reproduced.

[Per-case fixture/source manifest](../../testdata/compat/phase8-manifest.json).
Default deterministic integration tests exercise the same corpus against native
Go, exact event/argument assertions, normalization boundaries and manifest pins.

## Commands / gate behavior

```sh
# No accounts, Rust provisioning or Docker needed for default checks:
make check
make build
git diff --check

# Explicit network/toolchain provisioning into task-owned scratch:
python3 tests/e2e/build-atuin.py /path/to/pinned-atuin /scratch/cli
python3 tests/differential/build-oracle.py /path/to/pinned-server /scratch/oracle
ATUIN_E2E_MANIFEST=/scratch/cli/manifest.json \
ATUIN_ORACLE_MANIFEST=/scratch/oracle/manifest.json make compatibility
```

Default `go test` explicitly **SKIPs** the external CLI/oracle tests when artifact
manifests are absent, with BLOCKED reasons visible under `-v`. That is not a
passed compatibility gate. `make e2e`, `make differential` and `make compatibility`
**FAIL** without prerequisites. The compatibility workflow now runs secret-free
on `pull_request`, `push` and manual dispatch (NOT `pull_request_target`), with
an aggregate depending on both Go checks and actual compatibility. It has not
run for these changes; no CI pass is claimed. Administrators must require
`Phase8 validation aggregate` (and applicable existing checks) in branch rules;
repository code cannot enforce that external setting and does not modify it. Normal
Go CI retains all existing checks, 5s fuzz smoke and 2m nightly budgets, and adds
formatting coverage for tests. No account secret is used by either job.

Validation at the writer checkpoint:

- `make check`: PASS after a test-helper ST1013 numeric-HTTP-status fix; includes
  unit/race/vet/Staticcheck/nilness/govulncheck (no vulnerabilities) and all four
  unchanged fuzz targets. Final post-edit validation is recorded below.
- `make build`, `git diff --check`: PASS.
- Source-pinned differential hard gate: PASS (14 cases, exact counts).
- Full real-CLI hard gate: PASS (initial 19 cases); separate new-process resume
  case PASS. Final combined 20-case sweep is recorded below.
- Targeted race x10: integration, application, HTTP server, native provider PASS;
  fixture helper package has no separate tests. External PTY/oracle race sweep
  and final full checks are recorded below.

### Preserved initial failures / harness corrections

These failures were not filtered, retried internally or counted as passes:

- Initial fixture transport omitted the required message-added event before a
  Responses text delta, and initially used a nonprivate temp-store parent.
  Fixed fake event sequencing and 0700 fixture parent; no runtime change.
- Initial oracle build hit real Debian security-package 404s
  (`/tmp/phase8-oracle-build.log`). The pinned signed snapshot build then passed.
- Docker 29's internal network did not publish requested loopback ports
  (`/tmp/phase8-oracle-test1.log`); an isolated diagnostic confirmed null port
  mappings. Removed all publications and moved only the oracle HTTP helper
  into the internal bridge. No host-network/nonloopback workaround.
- Original fake CC selector assumed user content was a plain string; pinned
  projection used structured content. It now finds the synthetic case marker
  in structured user content, without changing native/reference requests.
- First CLI sweep failed output correlation and waited for an edit preview not
  shown by this TUI (`/tmp/phase8-cli-all1.log`). Later focused parallel runs
  exposed incremental redraws, not whole-screen repeats. The edit now waits for
  its actual permission label. Final executed parallel coverage uses a read
  (real permission) plus an auto-permitted registered skill, asserting both
  distinct call/result IDs; it does not infer a second prompt from ambiguous
  terminal redraw fragments. The differential parallel fixture independently
  compares two read calls exactly. No parallel batch/count assertion is skipped.
- Output setup had falsely accepted **"Daemon is not running"** as ready and
  raced first-run migrations. The captured diagnostic
  `/tmp/phase8-cli-output5.log` reports `no such table: events`. This is not a
  Go protocol/runtime bug claim. Initialization is now serial, readiness requires
  the exact source's running/healthy output, terminal size is set pre-exec and
  real daemon storage acknowledgment precedes retrieval. Focused output x3
  passed (`/tmp/phase8-cli-output6.log`). No arbitrary sleeps/retries hide a
  failed inference, migration or storage assertion.
- The first full external race x2 sweep (`/tmp/phase8-e2e-race2.log`) FAILED:
  missing model-picker text / CLI exit in several cases. The harness had used
  visible text as an idle witness, although pinned `app.rs` permits text before
  FSM `done` (Esc then cancels rather than exits). Exact historical key/event
  interleavings were not instrumented and are not claimed. Successful text
  steps now require the source's `Responded in` idle banner AFTER that unique
  answer before slash/exit keys; model selection also asserts the actual second
  request's `config.model`. No runtime change or error filtering. Focused model/
  resume/write/buffering race x3 PASS; final full external race results below.
- Strengthening the raw unknown-usage cost assertions failed
  (`/tmp/phase8-usage-policy-final-race.log`, then
  `/tmp/phase8-unknown-cost-diagnostic.log`): reference costs were strings, not
  JSON null. Source `http/streaming.gleam` passes `dynamic.nil()` into
  `ffi/atuin_ai_json_ffi.erl`'s OTP `json:encode/1`. An isolated no-network eval
  of the actual pinned image confirmed `json:encode(nil)` → `"nil"`, while
  `json:encode(null)` → JSON null (`/tmp/phase8-reference-nil-codec.log`). The
  explicit fixture policy now asserts this difference; it is NOT normalized
  away and native's existing null policy is unchanged. The earlier combined
  command `/tmp/phase8-compatibility-final3.log` therefore FAILED its oracle
  portion (20-case CLI portion passed); it is not counted as final success.

Historical logs/artifacts are retained under `/tmp/phase8-*` and the OLD
`/home/pi/.cache/atuin-ai-phase8/` scratch. They are not the current artifacts;
use the `-fixed-cli` / `-fixed-oracle` paths in final acceptance below. Test containers/networks
are removed by cleanup; no unrelated images/caches were deleted or pruned.
Independent review, delivery/CI and production-live/resource gates remain
separate from the writer's local synthetic passes.

## Historical pre-review local passes — NOT REVIEW CLEARED

These observations predate the critical ancestor/cache/assertion review findings.
They are preserved, NOT presented as current isolation/compatibility gate proof:

| Command / scope | Outcome / retained local log |
| --- | --- |
| `make check` | PASS, `/tmp/phase8-final-validation.log`; all unit/race/vet/Staticcheck/nilness/govulncheck checks and four unchanged 5s fuzz targets; no vulnerabilities found. External artifact tests explicitly skipped in this default run, not counted as passes. |
| `make build`; `git diff --check`; formatting | PASS, `/tmp/phase8-final-validation.log` ends with `FINAL_CHECK_BUILD_DIFF_EXIT_0`; `gofmt -l cmd internal tests` empty. |
| Both manifests set, `make compatibility` | PASS, `/tmp/phase8-final-compatibility.log` (explicit zero-exit marker): **20 actual CLI cases** and **14 source-built reference cases**, including raw unknown token/cost policy and exact generation counts. CLI about 49s; oracle about 2.5s. |
| Full `go test -race -v ./tests/e2e -run '^TestPinnedCLI$' -count=2 -timeout=5m` | PASS, 40 case executions, `/tmp/phase8-e2e-settled-final-race2.log`. This precedes the final explicit daemon PID/socket and stronger failure-flag assertions; those final additions passed the targeted sweep below AND final full hard gate. |
| Final `atuin_output`, `permission-denial`, `execution-failure` external race x2 | PASS, `/tmp/phase8-final-daemon-failure-race2.log`; actual owned daemon PID/socket, positive captured output, exact `Exit code: 7` and CLI `is_error:true`. |
| Final seeded-history unique-ID assertion race x2 | PASS, `/tmp/phase8-history-id-race2.log`; actual returned `History ID` must match the CLI-seeded unique ID, so a no-results/query echo cannot pass. |
| Final oracle + deterministic integration race x2 | PASS, `/tmp/phase8-explicit-nil-policy-race2.log`; actual final image and explicit five token/four cost assertions. |
| Integration/application/HTTP/native-provider race x10 | PASS, `/tmp/phase8-targeted-race.log`; additional fixture package has no separate tests. |
| No-manifest `go test -v ./tests/e2e ./tests/differential` | Explicit BLOCKED skips, `/tmp/phase8-default-skip.log`. |
| No-manifest `make e2e` / `make differential` | Both correctly FAIL nonzero, `/tmp/phase8-blocked-gates.log`; missing prerequisites cannot become a green hard gate. |

Final recheck: all three original source HEADs still match their separate pins,
all original worktrees clean. No task PTY/daemon/capture process or internal
Docker network remains. Scratch artifacts/source-built images are retained for
review; no unrelated data/global cache pruning. Final sampled free disk: about
11 GiB repository filesystem, 6.2 GiB `/tmp` tmpfs.

The initial checkpoint was submitted for review, which found the gaps above.
No commit/push followed. Production live login/catalog, Phase 9 and release gates
remain blocked; no account/model eligibility follows from these synthetic runs.

## Historical review-fix snapshot / corrected writer validation

Review found real HARNESS gaps, not a basis for changing production policy:

- Ancestor context and embedded shell interpolation were not confined by HOME/
  XDG alone. Actual CLI runs paused. The new fd-pinned filesystem/PID boundary
  was installed and synthetic namespace tests passed BEFORE actual CLI resumed.
  External context/project/skill and command markers are checked in the real
  picker test; symlinks, replaced ancestor paths, ignored SIGINT, preparation/
  PTY interruption, wrapper death and launcher-parent death have regressions.
  Do not infer absence of historical reads/execution or invent an incident.
- Installed markers, retained Cargo config/target inputs and source overlays are
  gone from the trust path. Both builders invalidate stale manifests, use new
  private run directories and verify complete source inventories before/after.
  Official cached archives are checksum-verified every use; toolchain installs,
  Cargo HOME and targets are fresh. Cargo runs in its own namespace, with no
  host config/wrapper/ancestor inputs. The complete installed toolchain inventory
  is checked unchanged after compile. Corruption/config/extra `.ex`/mode/stale-
  manifest regressions pass. Local manifest consistency is NOT independent
  producer attestation; an adversarial same-UID producer is not authenticated.
- The held function item now reaches `response.output_item.done` BEFORE the
  visible witness while overall completion remains withheld. Proxy bytes must
  contain ZERO executable frames before release; after release the permission
  match must be fresh. Exactly one resulting frame/write/result is required.
  A deliberately premature split frame regression is rejected.
- Fakes independently validate exact configured model, ordered enabled schema
  declarations and full ordered dialect-specific history/correlation BEFORE
  emitting canned success. No production decoder/projector/registry builds
  expectations. Mutation tests cover dropped/reordered tools/history, constraint/
  required/description-parameter loss, IDs and trailing JSON. Mixed text/calls,
  text/result/text, parallel results, failed outputs, legacy calls and historical
  suggestion dialects are covered. Prompt prose is not byte-compared; context
  XML/data and date FORMATS are validated (fixed native clock versus reference
  host UTC date may differ). No SSE behavior is normalized for this difference.
- Raw UUID/header/done/explicit-continuation validation precedes normalization.
  Missing/null/empty/malformed/mismatched/changing IDs fail. Null/empty *request*
  session IDs intentionally mean a new session. All raw usage keys/values are
  checked. Intentional policies now assert complete expected error/call/order/
  status/done payloads; risk reconciles only the asserted risk/notes delta and
  compares every remaining field. Mutation regressions reject unrelated changes.
- Ownership names/run+lease labels and a separate-process-group watchdog are
  registered BEFORE Docker creation. Begin/end lease records distinguish an
  interrupted create; EOF/parent death triggers only scoped deletion, with a
  30s late-create drain for uncertain operations. A killed-parent synthetic
  partial/late-create regression proves unrelated resources survive. Workflow
  always-cleanup additionally scopes to its unique run label. All Docker clients
  use the explicit local Unix socket and isolated HOME/config, not normal Docker
  auth/context stores. No global prune. Unresponsive Docker/cleanup remains a
  failing gate, not an unlimited cleanup guarantee.
- CI is now secret-free PR + push + manual, not pull_request_target or manual-
  only. The aggregate fails unless both Go and actual compatibility succeed;
  cancellations/skips do not pass. External admin branch/release rules must
  require `Phase8 validation aggregate`; none were changed automatically.
  Compatible Bubblewrap/user namespaces are mandatory runner prerequisites.
  No corrected CI result is claimed before an actual run.

### Corrected artifact/build evidence

- Fresh Atuin cold build **10m53s**, default AI/daemon/PTY features and locked
  source unchanged; binary SHA256
  `b940cd03e687cb4a8e6d227c57d1ca17101308b52efb65a1ef211bc8e2d0ac11`.
- Fresh original source/core image
  `sha256:74e20df94de3e6fcd31b0d0a53026fd5dde251934acc688da87058355b49af61`.
- Use corrected manifests, not the historical marker/overlay-era artifacts:
  `/home/pi/.cache/atuin-ai-phase8-fixed-cli/manifest.json` and
  `/home/pi/.cache/atuin-ai-phase8-fixed-oracle/manifest.json`.
  Gates check archive/recipe/helper/flags/component/source-unchanged proof as
  well as revision, binary hash/image ID. [Full provenance](../provenance/phase8-tooling.md).

### Preserved failures while fixing the harness

1. `/tmp/phase8-review-cli-build.log`: fresh namespace initially lacked `cc`
   because the host alternative pointed into unmounted `/etc/alternatives`.
   Explicit `/usr/bin/gcc`/`g++` linker paths fixed build setup; no source/features
   patched. Only that task-owned FAILED run directory was removed; official
   checksum-verified download archives/logs retained. Fresh build2 passed.
2. `/tmp/phase8-review-harness-unit1.log`: the synthetic `/usr/bin/true` stub
   was itself a symlink, correctly rejected by nofollow binding. Unit stub uses
   its resolved system executable; actual pinned artifact must remain regular.
3. `/tmp/phase8-review-harness-unit2.log`: a test selector matched both the
   Python driver and bwrap reaper argv. It now checks argv0 and closes pipes.
4. `/tmp/phase8-review-compatibility1.log`: prerequisite interruption tests
   failed BEFORE real CLI cases. Background launch may inherit ignored SIGINT;
   a controlled ignored-SIGINT regression now tests explicit driver handlers /
   launcher reset. Exact old signal disposition was not instrumented and is
   not retroactively claimed. No Go runtime bug or filtered retry is asserted.
5. `/tmp/phase8-review-check1.log`: new test-oracle error text triggered ST1005;
   lowercased the helper message only. Checks/budgets were not weakened.

Corrected final commands/results are recorded below. Production/live/catalog,
Phase 9, actual CI and independent re-review remain separate gates. No commits
or pushes and no client/reference application-source patches were made.

### Corrected writer results (2026-10-17 UTC)

- `make check`, `make build`, formatting and `git diff --check`: **PASS**;
  `/tmp/phase8-review-frozen-validation.log` ends
  `REVIEW_FINAL_FROZEN_CHECK_EXIT_0` (final source, including the default-only
  non-Linux pidfd-regression guard). No fuzz budget/assertion/devtool pin
  was changed. Default external-artifact skips remain distinct from hard gates.
- Corrected fresh manifests set, `make compatibility`: **PASS**, all **20 real
  namespace-confined CLI cases** and **14 original-source oracle cases**;
  `/tmp/phase8-review-last-compatibility.log` ends
  `REVIEW_LAST_COMPATIBILITY_EXIT_0`. Mandatory confinement/interruption/parent-
  death regressions run before the actual client. All seven tools include
  positive owned-daemon output retrieval. No scenario retries/filtering.
- Full external + integration/native runtime race x2: **PASS**, including 40
  real CLI CASE runs (plus preparation/continuation invocations) and 28 oracle cases, `/tmp/phase8-review-race2.log`.
  Final complete-function-item (`output_item.done`) barrier/error strengthening
  passed focused external race x2 (`/tmp/phase8-review-complete-call-race2.log`)
  AND the final full hard gate. Final local-socket/independent-group lease,
  projection/policy/session/barrier/integrity helpers passed race x2 in
  `/tmp/phase8-review-last-targeted-race2.log`.
- All offline harness regressions: **PASS**,
  `/tmp/phase8-review-harness-unit5.log`; the final explicit local-Docker scoping
  variant also passed `/tmp/phase8-review-client-scoping-unit.log` and default
  Go checks. Controlled background ignored-SIGINT is now part of the regression.
  Only the auxiliary default pidfd watchdog test is Linux-conditional; hard
  compatibility still requires Linux/Bubblewrap and fails without it. No macOS
  execution or Phase 9 readiness is claimed.
- Original Atuin/server/core HEADs remain pinned and worktrees clean. No task
  PTY/daemon/capture/watcher process or owned Docker network remains after
  cleanup. About **8.1 GiB** repository disk / **6.2 GiB** tmpfs free at final
  sampled check. Only task-owned failed build data / generated Python cache
  files were removed, never unrelated caches/images or global prune.

**Historical pre-final-review snapshot (superseded by final acceptance below).** Actual GitHub
CI/runner namespace availability and administrator-required branch checks have
NOT been demonstrated or configured. Production live login/catalog, Phase 9 and
release gates remain blocked. Historical unconfined passes are NOT isolation
proof; there is no inferred incident or retrospective no-leak claim.

## Narrow normalization / CI prerequisite follow-up (2026-10-17)

CLI confinement/build/buffering/cleanup re-review CLEARED with no new blockers.
Independent Part 1 default checks passed; independent actual CLI/race verification
was still pending when this follow-up was authorized. Oracle re-review verified
all original fixes but found a remaining P2: adjacent text merging discarded a
subsequent chunk's non-content fields. Normalize now merges ONLY two content-
only payloads (`len == 1`, string content on BOTH chunks). Prior/subsequent/
middle chunk metadata and ordering remain intact. New direct regressions plus
positive AND intentional-policy comparison mutations reject the original loss.
No runtime or compatibility policy changed.

The other known prerequisite was NOT a CI pass: ubuntu-latest did not provision
Bubblewrap 0.11.1 or demonstrate usable namespaces. The compatibility job now
uses ubuntu-24.04, installs bounded signed-OS build prerequisites on the
EPHEMERAL hosted VM, builds the checksum-pinned official release into a private
prefix, verifies its complete installed inventory/recipe/version/non-setuid
mode, and requires an actual same-arguments filesystem/PID/FD namespace probe
before any CLI run. It does NOT assume the distro bwrap version. OS package
versions and tool executable hashes are recorded; source checksum is
`c1b7455a1283b1295879a46d5f001dfd088c0bb0f238abb5e128b3583a246f71`.

**New privileged CI setup (subsequently CLEARED final narrow review):** when the hosted kernel's
AppArmor unprivileged-userns restriction is 1, the job loads ONLY an exact-path
profile for this job's verified non-setuid Bubblewrap. It grants `userns`, not
root. No global sysctl/AppArmor disable, broad wildcard profile, privileged
host mount, sandbox bypass or developer-host install/policy change is used.
Profile identity and cleanup receipt precede load; an always-step unloads ONLY
that run/attempt's named profile. Helper refuses developer/self-hosted execution.
The mandatory namespace probe must still succeed; setup cannot turn a missing
prerequisite into a skip. Branch protection remains external/unmodified.

The cleared CLI harness changed ONLY to select the explicitly verified test-tool
path: `confinement.py` uses `bwrap_tool.resolve`, `cli_test.go` whitelists the
manifest path into the existing sanitized launcher environment, and the Atuin
builder selects the same tool. PTY, prep, prompts, tools, lifecycle and namespace
arguments are unchanged. Without the opt-in variable the reviewed system path
remains `/usr/bin/bwrap`; invalid configured manifests fail, never fall back.
Because the build recipe changed, a fresh complete Atuin rebuild is required,
not a metadata-only relabel of the prior artifact.

### Preserved new provisioning observations (not CI execution)

- Build 1 linked against an extracted Ubuntu libcap .pc `libdir=/usr/lib64`,
  while its files were multiarch. The DEVELOPER-OWNED extracted dependency stage
  now supplies that symlink; no installed package/header/source was patched.
  `/tmp/phase8-bwrap-source-build1.log` preserves the failure.
- Build 2 exposed tuple/list JSON inventory comparison in the NEW selector.
  Canonical JSON inventory comparison fixed it; a serialized positive regression
  now prevents rejecting every real manifest. The failed manifest was invalidated;
  `/tmp/phase8-bwrap-source-build2.log` is retained, not retried unchanged.
- Build 3 verified source/installation/version successfully. Its unprivileged
  prefix namespace probe hit this DEVELOPER host's AppArmor UID-map restriction
  (`setting up uid map: Permission denied`). This is a prerequisite FAILURE,
  not a pass or reason to weaken confinement. No developer profile/sysctl was
  changed, and no actual Atuin was run through that failing prefix. A subsequent
  prefix-only synthetic confinement regression likewise failed at namespace
  setup (`/tmp/phase8-bwrap-prefix-confinement.log`), not inside Atuin.
- New profile logic passes offline mocked install/partial-failure/scoped-removal/
  tamper/developer-host rejection tests. Actual AppArmor parser syntax-only
  `-Q -T` passed (`/tmp/phase8-bwrap-profile-syntax.log`), with NO kernel load.
  The privileged CI install/remove path had NOT actually run locally; final
  code review subsequently CLEARED it. Actual hosted CI remains a separate gate;
  no hosted namespace success is inferred from review.

Local new dependency provisioning was download/extraction ONLY into owned
scratch, never developer-host apt install. Source/tool/license/checksums and
current results follow in [tooling provenance](../provenance/phase8-tooling.md).
No commit/push, real credentials/login, Phase 9 or branch settings changed.

### Narrow-follow-up writer results

- `make check`, `make build`, `git diff --check`: PASS;
  `/tmp/phase8-narrow-validation1.log` ends `NARROW_CHECK_BUILD_DIFF_EXIT_0`.
- New normalization + policy comparison mutations, prefix manifest/version/
  corruption/setuid/source-recipe rejection, profile hosted-only/exact-path/
  partial-setup/removal/tamper mocks and existing integrity/watchdog tests: PASS;
  `/tmp/phase8-narrow-harness1.log`, `/tmp/phase8-narrow-unit1.log`.
  Targeted race x2 PASS: `/tmp/phase8-narrow-unit-race2.log`.
- Corrected actual original-source oracle **14/14 PASS**;
  `/tmp/phase8-narrow-oracle1.log` ends `NARROW_ORACLE14_EXIT_0`.
- Recipe selection change triggered a REAL fresh Atuin source/toolchain/Cargo/
  target rebuild: **11m06s**, `/tmp/phase8-narrow-cli-rebuild.log` ends
  `NARROW_CLI_REBUILD_EXIT_0`. Binary SHA256 remains byte-identical
  `b940cd03e687cb4a8e6d227c57d1ca17101308b52efb65a1ef211bc8e2d0ac11`;
  current recipe SHA256
  `52a7e17c575a19c29b3730b527e8aaf0461f251b1b82c9d83654189531250cdf`.
  Existing corrected manifest path now records that real new build.
- Current-source actual CLI **20/20 PASS**, required confinement regressions also
  PASS; `/tmp/phase8-narrow-cli20.log` ends `NARROW_CLI20_EXIT_0`. Real CLI
  race x2 PASS (40 case runs): `/tmp/phase8-narrow-cli-race2.log`, 98.448s.
  These use the reviewed SYSTEM bwrap, NOT the developer-blocked source prefix;
  no claim is made that the new CI exact-prefix kernel path has passed locally.
- AppArmor profile real syntax-only parse PASS; actual privileged install/remove
  remains UNEXECUTED (mocks are not kernel verification). Source prefix version/
  inventory build PASS; its developer-host namespace probe remains BLOCKED.
  No fallback, disable, sysctl change or developer sudo setup forced green.

### Final independent acceptance / authorized delivery (2026-10-17)

ALL final independent reviews CLEARED: confinement/cache/buffering/early child
cleanup; complete oracle projections/sessions/policies; lossless text metadata
normalization; owned Docker cleanup; and hosted-CI verified Bubblewrap selection /
exact-path AppArmor setup. No unresolved review blocker remains in this code
checkpoint. No runtime-policy or upstream application-source behavior changed.

Coordinator-reported FINAL independent verification on CURRENT corrected paths:

- **20/20 actual CLI + 14/14 original oracle, no skips**.
- **CLI race x2: 40 case runs; oracle race x2: 28 case runs**, all passed.
- **Unfiltered integration race x2**, plus earlier **integration race x10**, passed.
- Default **make check / build / diff checks** passed independently.
- An initial verification attempt using an OLD manifest failed 0/20 because its
  recipe/proof no longer matched. That is a valid old-artifact rejection, not a
  runtime bug or a result to suppress; corrected-path verification passed.

Current manifests (not the legacy non-fixed scratch paths):
`/home/pi/.cache/atuin-ai-phase8-fixed-cli/manifest.json` and
`/home/pi/.cache/atuin-ai-phase8-fixed-oracle/manifest.json`. The client remains
`b940cd03e687cb4a8e6d227c57d1ca17101308b52efb65a1ef211bc8e2d0ac11`;
reference image remains
`sha256:74e20df94de3e6fcd31b0d0a53026fd5dde251934acc688da87058355b49af61`.
Local manifests establish consistency, NOT independent producer attestation.

Normal focused main commit/push is now authorized. Delivery must watch ALL
workflows for that EXACT pushed SHA: existing Go Checks, compatibility Go job,
source-prefix Bubblewrap version/inventory + real namespace probe, fresh Rust /
reference builds, actual CLI/oracle gates, validation aggregate and always cleanup.
A green old Go-only run is insufficient. The reviewed privileged hosted-kernel
path remains UNVERIFIED until its actual CI job passes; no developer policy is
changed to force that outcome. Preserve and diagnose any first CI failure before
code changes, never blind-rerun or weaken isolation. Branch-protection admin
settings remain external/unmodified. Report delivery SHA/URLs outside this
pre-commit snapshot rather than claiming unexecuted CI results.

Phase 9 can begin only after exact-HEAD CI is green; no Phase 9 work is included
here. Production login/live lifecycle and eligible catalog remain **NOT PASSED**;
probe generation 3 is separate. No real credentials/provider calls or release
clearance is inferred from these synthetic compatibility results.
