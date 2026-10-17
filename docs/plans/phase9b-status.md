# Phase 9b — offline shipping resource / lifecycle review checkpoint

> **Bundled catalog correction (2026-10-17):** [authorized policy](bundled-catalog-policy.md)
> removes 15 automatic metadata GETs (one per fresh catalog/process lifetime),
> not generation work: full suite still requires 446 generations, peak four,
> unchanged barriers/budgets and real 60s/300s defaults. Expected automatic
> catalogs is now zero; explicit TLS catalog wire tests remain. Prior counts
> below describe prior source only. Current Mac/both-model native/live gates
> remain release blockers; no publication is authorized.

## Boundary

Base: CLEAN main `edfa90452922cc2792e6e001277821bfcad6da3d`.
Coordinator supplied ALL exact-HEAD Phase9a GREEN receipts: packaging
`39004040516`, Go `39004040656`, compatibility/aggregate `39004040576`.
The user has the development Mac artifact/hash/checklist and volunteered native
execution; outcomes remain **PENDING**. No tag, release or public publishing is
authorized. Production login/live lifecycle and eligible discovery remain
**NOT PASSED**; retained-probe generation3 and real credentials are never read.

Phase9b adds tests, explicit hard targets, a secret-free CI job and evidence
instructions ONLY. No production code, dependency, auth/store/model policy,
upstream URL, timeout, configuration knob, management listener or profiling
endpoint changes. BOTH focused independent reviews now CLEARED measurement and
isolation fixes; independent local validation/full resource verification PASSED.
Code checkpoint delivered as `a5ec69f591a1fb83644171713183029d378f8634`.
Its Go CI FAILED Formatting; other workflow outcomes are not recorded by this
correction. The mechanical follow-up below awaits review BEFORE another commit/
push. The all-four CI checkpoint is NOT PASSED; Phase9c/publication unauthorized.

## Delivery CI formatting follow-up — correction awaiting review

Exact-HEAD [Go39009816461](https://github.com/Jghh42/atuin-ai/actions/runs/39009816461)
failed the Formatting step `test -z "$(gofmt -l cmd internal tests)"` (exit1).
Preserved log: `/tmp/phase9b-ci-39009816461-format-failure.log`. The full local
format-list command named ONLY `tests/resourcehelper/main_test.go`:
`/tmp/phase9b-ci-format-full-before.txt`; original formatter diff is retained at
`/tmp/phase9b-ci-format-first.diff`. Prior local make-check/build/diff passed but
DID NOT include CI's formatting check; they did not establish formatting success.
The other THREE workflows' outcomes are unknown in this correction, not GREEN.

Applied gofmt ONLY to that helper TEST file. The result is byte-identical to
running gofmt on its committed a5ec version; only whitespace/statement layout
changed, no assertions,30ms test deadline, workload or production code changed.
`Makefile` also adds a minimal `fmt-check` prerequisite to `check`, running the
EXACT CI format command across cmd/internal/tests without automatically editing
files. This is a proposed build-gate change for quick independent review, not
an already accepted follow-up. A controlled negative before formatting proved
this gate rejects the known bad file (`/tmp/phase9b-fmt-check-negative-first.log`);
the same full CI command is now empty/PASS.

Focused helper tests and race20 PASS (`/tmp/phase9b-format-fix-helper.log`,
`/tmp/phase9b-format-fix-helper-race20.log`); new make-check, build, exact full
format command and diff check PASS (`/tmp/phase9b-format-fix-check.log`,
`/tmp/phase9b-format-fix-build.log`). No seven-minute local resource rerun or new
package was performed for this formatting-only test change. The accepted085
package/report below remain immutable evidence for their PRIOR inputs, NOT a
new SHA: test-format/build-gate/documentation changes alter holistic inventory.
After review/authorization, a NEW exact-SHA resource CI must run the entire
suite/count/provenance/cleanup gates. No old workflow rerun, force/amend/tag/
publication or branch-setting change; no commit/push of this correction yet.

## Final independent acceptance / authorized focused code delivery

Coordinator reported BOTH focused reviews CLEARED: eight verified public inputs
in a private read-only snapshot; constructor ownership/rollback; actual namespace
confidentiality and CONNECT/TLS wire regressions; all-four CI failure propagation;
retention boundaries and observed sampling gaps. Independent fast validation
PASSED make-check/build/diff,10+10 tests and helper/http cancellation race20.

Independent verifier then built BOTH minimum targets from TWO absolute paths,
ran the ENTIRE resource suite to **PASS**, and repeated36 packaging tests,
32 artifact mutations, actual shipping smoke and the current-source consumer
contract AFTER the run. Immutable measured inputs:

- Source inventory `08573867a21ea1d37fe03d43ef95e4fe6a3f3c7bfa093392afa810094beb0046`
  (221 files, honest dirty=true at base `edfa904`).
- Package `dist/phase9b-independent-final-package/`; evidence
  `dist/phase9b-independent-final-evidence/`.
- Measured Linux raw binary SHA256
  `7cee15057504a8fd5db74ee273bcb2afee5d45a7f0253d1dc93ddb10783f0efa`.
- Full report SHA256
  `f246484db9d32681579bbf1a33e1ca0b21a4eebf8efa0ae52fb958e7a39a7a40`.

| Shipping scenario | Conservative HWM KiB | Project hard gate | Project soft goal | Max observed sample interval |
| --- | ---: | --- | --- | ---: |
| Warmed idle | 12416 | <=64MiB PASS | <=32MiB PASS | 20.624ms |
| Four normal | 15788 | No separate project normal target | <=128MiB PASS | 20.333ms |
| Four near-limit | 109884 | <=256MiB PASS | Not specified | 57.957ms |

The128MiB four-normal target is **SOFT**, not hard. Sampling requests20ms but
actual gaps above are observed, not guaranteed; conservative kernel HWM remains
independent peak evidence. Four exact4193280-byte valid bodies, fifth503,
four concurrent native generations and four parser witnesses PASSED. Full suite:
446 generations,15 catalogs,23 owned processes; all handlers/samplers/processes
joined, private source/runtime trees removed. Shipping100 completions AND100
disconnects plus nonreading cancellation passed; companion repeated those counts.

Actual default inactivity60.045s / generation299.998s passed. Shipping nonreading
cancellation~5.077s is black-box evidence, not an invented write-error timestamp.
Companion observed actual DEFAULT5s partial write32768/61481 bytes, then upstream
EOF265037ns after the error. Both signals/empty partial body/four active work,
listener closure, bounded drain and same-store restart PASSED. Eleven matched
companion GC snapshots: goroutines6, FDs9, sockets1; heap+101664 bytes/+397 objects
within UNCHANGED512KiB/512-object finite bounds. This is NOT proof of a plateau
or zero leakage. No companion runtime/socket instrumentation is attributed to
the unchanged shipping executable.

This documentation acceptance receipt changes holistic source identity; later
commit/linker identities change it again. The085 package/report are valid ONLY
for their immutable prior inputs, NEVER relabeled as the delivery commit.
Fresh exact-commit CI DEVELOPMENT artifacts must carry their own actual source
identity and full resource/count/provenance/cleanup assertions. Local acceptance
is not hosted CI success or publication approval. Root coordinator monitors ALL
FOUR exact-HEAD workflows after push; this writer does not wait for CI or start
another phase. Native Mac user outcomes remain PENDING; production live lifecycle
and eligible catalog remain NOT PASSED; public release stays BLOCKED.

## Historical focused isolation blocker — corrected and independently CLEARED

Independent measurement review CLEARED the stated finite bounds, not a zero-leak
claim. Isolation review found a BLOCKER: the previous read-only **whole working
repository** bind exposed `.git` and ignored/private material. Read-only was
integrity protection, NOT confidentiality. This supersedes ALL earlier Phase9b
isolation/full-acceptance claims, including the historical writer PASS report for
source inventory `5fbf9bd2bdfac3014e1c736a7ea1fa4ac4efe1778c64f26358c463b76692361d`.
That package/report remains immutable historical numerical evidence, marked
`unsafe-snapshot.superseded`, NOT a currently accepted resource artifact. No
private-data access incident is inferred merely from exposure.

The independent verifier intentionally STOPPED its all-case run for safety;
`dist/phase9b-independent-review-evidence/report.json` remains **RUNNING/PARTIAL**,
NOT a completed full PASS, and all owned resources were cleaned. Previously
completed checks/build/package and selected resource cases are not a full
aggregate. This was a safety interruption, not provider failure or a retry.

The sole-writer focused patch replaces the whole checkout bind with a private
closed EIGHT-file source snapshot, every file checked against the measured
package's verified actual source inventory. Outer FULL source/HEAD/package truth
remains separate; inner no-Git checks verify the exact read-only subset. No `.git`,
ignored/private paths or writable alias to the snapshot. Source and runtime trees
are private and removed even on setup failure. Actual synthetic namespace
confidentiality tests use the SAME mount builder and OWN `.git`/Git-ignored
sentinels; required public inputs/binary stand-ins remain available. No real
private directories are read or used as fixtures.

Process ownership now registers immediately after spawn. Any later constructor
failure terminates/reaps only its owned group, closes all descriptors/streams,
and joins any sampler. Failure-injection tests include discovery, pre/post
sampler start, actual Thread.start, spawn and second-pipe failure, with an
unrelated process proven untouched. Actual namespace CONNECT/TLS wire negatives
and positive real-CA catalog test enforce authority/SNI/Host/framing and deny all
DNS/non-fixture dials. Additional boundary tests pin the UNCHANGED retention
allowances/+1 and solely monotone-tail rejection. Reports now expose maximum
OBSERVED sampling interval, not a guaranteed20ms cadence.

Focused tests:10 fast invariants and10 namespace confidentiality/ownership/wire
tests PASS. Fresh bounded make-check/build/diff and helper race20 receipts are
`/tmp/phase9b-isolation-fix-final-check.log`,
`/tmp/phase9b-isolation-fix-make-build.log`,
`/tmp/phase9b-isolation-fix-helper-race20.log`; fast namespace output is
`/tmp/phase9b-isolation-fix-final-namespace.log`. These are NOT a full resource
run or independent clearance. The first NEW invariant run failed because Python mkdir(parents=True)
created intermediate755 directories under ordinary umask; the builder now
explicitly creates EACH new snapshot ancestor0700, without chmod/policy bypass.
First failure `/tmp/phase9b-isolation-fix-unit-first.log` is retained. At that
checkpoint no new package/full rerun was permitted before critical clearance.
The subsequent CLEARED reviews and fresh independent package/full PASS are
recorded above. `/tmp/phase9b-review-handoff.md` preserves that earlier held-source
checkpoint; unsafe prior artifact identities remain superseded, not relabeled.

Required delivery/release CI is independently ALL FOUR exact-HEAD families:
Go checks, Packaging consistency, Phase8 validation aggregate AND the new
Phase9b resource validation aggregate. Resource aggregate rejects failure/skip/
cancellation, and the job requires present full PASS/count/cleanup receipts.
Compatibility aggregate alone is NOT overall clearance. Branch required-check
administration is external and has not been changed/claimed.

## Actual pipeline and measurement (current corrected design)

[Reproduction, exact workloads, sampling and trust boundaries](../provenance/resource-lifecycle.md).
The unmodified DEVELOPMENT-packaged Linux executable uses its native production
manager/store/catalog, full decoder/projector, native HTTPS Responses parser,
runner and default HTTP server. A separate deny-default CONNECT/TLS process
terminates only the fixed provider authorities locally, checks SNI/Host/route/
query, synthetic token/account binding, complete history, exact golden policy/
tool schemas and semantic request fields. It never forwards or dials a provider.
Fresh synthetic Manager.Login setup closes its lifetime store before serving.

An independent mandatory Bubblewrap network namespace contains server, fixture,
load generator and sampler: only usable loopback, no routes, remote-connect
negative witness. Fresh tmpfs filesystem root plus private HOME/XDG/temp avoids
host credentials and preserves the secure store's unchanged ancestry policy.
Ephemeral CA/private keys and fake records are removed, never uploaded.

Shipping RSS is externally sampled at a REQUESTED20ms period on the **actual
server PID**; maximum observed interval is explicitly reported (not guaranteed);
GNU time's post-exec child supplies independent final kernel maxRSS. Wrapper,
fixture and load memory are excluded. Shipping GOMEMLIMIT/GOGC are UNSET, no
race instrumentation or forced GC. The separately identified companion composes
`application.NewWithOptions` and `httpserver.NewWithRuntime` defaults, emits
numeric runtime/metrics/MemStats over inherited pipes, and alone forces matched
post-GC retention snapshots. Its bounded-buffer TCP listener separately observes
an actual DEFAULT five-second partial-write timeout; never attribute those
runtime numbers or socket changes to the shipping executable.

## Historical writer observations / SUPERSEDED unsafe-snapshot identities

Pre-final development snapshots were independently packaged by the existing
both-target/two-path flow; these are **not final-source artifacts** and must not
be relabeled. Retained reports/logs:

- `dist/phase9b-trial-evidence/report.json`: idle HWM12692KiB, four-normal
  15800KiB, four-near133332KiB; all hard/soft gates passed. Four exact4193280-byte
  meaningful bodies admitted before decode; fifth503, four concurrent native
  generations and four parser/text witnesses; each near completion event is
  1047552 bytes with32 valid buffered write_file calls, none executed.
- The same trial passed100 shipping completions AND100 shipping disconnects,
  plus a nonreading cancellation; companion separately passed the same counts.
  Shipping FD/socket/thread counts and companion goroutines/FDs/sockets stayed
  bounded. Trial's only remaining failure was a NEW test's escaped-newline
  assertion in over-event, fixed without changing runtime/budgets/assertion.
- `dist/phase9b-witness-evidence/report.json` is explicitly PARTIAL (selected
  cases, not a full-suite PASS). Companion100+100 and nonreading default write
  timeout passed: partial TCP32768/61481 bytes, deadline5s, timeout-error →
  upstream EOF280336ns. Goroutines stayed6, FDs9, sockets1. Matched post-GC heap
  change208480 bytes /408 objects is nonzero, nonmonotone and within the stated
  small noise bounds; this is not a mathematical zero-retention proof.
- Actual shipping DEFAULT header-open/body-idle inactivity:60.039s; heartbeat
  body to generation deadline:299.996s. Exactly one generic terminal error,
  one native generation and no retry in each case. No shortened defaults.
- Both SIGINT/SIGTERM passed warmed idle, empty idle, **observably consumed**
  empty-state partial body and four-active-generation cases. Listeners closed
  promptly, idle exit~0.011s, partial/four-active forced cancellation~5.00s,
  and NEW processes reacquired the same store lock. Four separate hard-reject
  cases passed:4194305-byte body413/no upstream; oversized SSE/arguments/33calls
  each terminal error with no executable calls and no retry.

The previous source receipt used DEVELOPMENT package/evidence destinations
`dist/phase9b-review-fixed/` and `dist/phase9b-review-fixed-evidence/`; BOTH are now
superseded by the isolation blocker above, not current handoff artifacts. A NEW
exact-source package is required AFTER focused re-review clearance. Its SHA256,
source inventory, all-case status, counts and independent verification belong
in the external handoff/report, not a self-referential source hash assertion.
A PARTIAL report or CI skip never clears the full gate. Full default run requires
exactly446 synthetic upstream generations and15 catalogs, with zero surviving
handlers/processes. Subsequent independent review/verification PASSED on085 as
recorded above; exact-HEAD hosted CI remains a separate pending delivery gate.

## Preserved first failures and diagnoses

No workload reduction, production fix, blind retry or budget increase:

1. `/tmp/phase9b-package-first.log`: accidentally generated `__pycache__` was
   correctly rejected by existing source safety. Removed only owned generated
   cache; subsequent commands use `-B`/`-I`.
2. `/tmp/phase9b-smoke-first.log`: a read-only host-root mount inside userns maps
   host-root ancestry to overflowuid; setup correctly failed the unchanged
   store policy. Replaced it with a fresh tmpfs root and narrow read-only tooling/
   source mounts, not a permission bypass or host-policy change.
3. `/tmp/phase9b-smoke2.log` plus its report: minimal root has no ambient OpenSSL
   config. Generator now explicitly uses `/dev/null` config; the subsequent
   TLS failure in smoke3 identified the consequently absent CA basicConstraints.
   Generator now explicitly marks CA:TRUE/keyCertSign, and actual native TLS
   successfully validates it. No global CA or TLS-verification weakening.
4. `/tmp/phase9b-smoke3.log`, `smoke4.log`, `development-first.log`: sampler FD
   permission at process exit and inherited pre-exec parent RSS. Controlled
   `/bin/true` zombie reproduced EACCES despite parent ownership. A denial is
   accepted ONLY after a bounded independently observed child exit; any live
   denial/background sampler failure fails. GNU time's post-exec server child
   replaces contaminated direct wait4 RSS, while outer wrapper usage remains
   explicitly NOT_SERVER. Actual server PID is observed, never guessed.
5. Development signals initially sent the same signal twice (explicit send plus
   finish), causing default second-signal termination. Finish now joins the
   already signaled child without sending again. Empty partial-body test was
   also strengthened with owned socket queue consumption before signaling.
6. `/tmp/phase9b-trial-first.log`: over-event test searched for literal backslash-n
   rather than newline. Corrected that assertion; oversized body/event/args/calls
   then all passed in `/tmp/phase9b-witness-first.log`. No production defect was
   established or changed.
7. `/tmp/phase9b-review-full-first.log` and `dist/phase9b-review-evidence/`
   preserve the first all-case run's NEW exact-schema fixture failure. The static
   tool goldens describe tool definitions, while Codex projection additionally
   supplies the already-reviewed `strict:null`. Expected wire tools omitted that
   field. Read `functionDefinition` in native request.go; corrected the expected
   exact shape to REQUIRE strict:null (not tolerate arbitrary extras), and added
   a strict:true negative mutation. No provider request/runtime change. Original
   `dist/phase9b-review/` is superseded for handoff, never relabeled.

Reports retain first errors without overwriting/retrying the workload. Fixture
handlers and samplers are tracked and boundedly joined before final background
error checks. Only owned process groups/fresh scratch are cleaned; no global
Docker/network/process pruning, sysctl, root install or developer policy change.

## Historical validation before the superseded package

Writer `make check`, `make build` and `git diff --check` PASS:
`/tmp/phase9b-make-check-first.log`, `/tmp/phase9b-make-build.log`. That historical full check
includes existing Go unit/race/vet/Staticcheck/nilness/govulncheck/four unchanged
fuzz campaigns,36 packaging tests and5 resource invariant tests (including
semantic mutations). Companion setup/TCP witness race x10 PASS. Final package
verification/32 real-artifact mutations and ALL-case resource report are recorded
outside this source snapshot so no later receipt silently changes artifact
identity. No independent review/verification or new CI pass is inferred.

## Release gates still open

Independent Phase9b review and local full verification are CLEARED/PASSED.
Native macOS user execution, production fresh login/live lifecycle, eligible
live catalog and ALL FOUR exact-HEAD delivery CI families remain open. Synthetic resource success does not prove provider acceptance, real-account
behavior, arbitrary hardware bounds, zero-byte leaks or public-release readiness.
No public release/tag/publication is made by scripts or CI.
