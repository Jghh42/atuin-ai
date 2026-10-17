# Native init — independent-review checkpoint (2026-10-17)

## Development delivery receipt — reviews and independent acceptance cleared

All THREE review areas are now CLEARED: auth/TUI and FS/config at402 (their
production behavior unchanged), and final native v258 service review at
`e74200ad62df2dbc5e6b16ed4834130cd6b6b1d0d1d8bae25ed37584998927d4`
(258 files). The independent verifier passed fast checks/build/race20/plain
CGO0 targets/PTY smoke AND full final acceptance on that exact source:

- Actual pinned CLI20 and source-built oracle14 PASS.
- Fresh BOTH-target/TWO-path DEVELOPMENT package,36 tests +32 mandatory mutations
  before/after resource execution, raw installer/relocated shipping smoke PASS.
- Actual packaged wizard detached no-TTY => zero state; private PTY missing-login
  No => empty-store lock only, with no login/provider/service/config mutation.
- ENTIRE resource gate PASS:446 generations,0 catalogs,4 peak concurrent,23 owned
  processes cleaned; all wrappers reaped/server PIDs gone/samplers joined and
  fixture handlers joined. Real timeouts60.0449s/299.9971s. HWM KiB idle12632
  (hard64MiB/soft32 PASS), normal16184 (soft128MiB PASS; additional256MiB safety
  guard PASS, NOT a new soft requirement), near148420 (hard256MiB PASS).

Immutable pre-delivery artifacts:
`dist/init-v2-independent-final-package.RCTzVn` (sourcee742 above), manifest SHA256
`9ab32b6425674202dfbe5d682079d0ccc551cf3f0fa09da8b5066e2fa7bc7917`;
Linux raw `cc85bfa5fd8498eea5ab92931d58374377465bcf87ee1947ba77e159dc16d3ab`,
Darwin raw `c46ea93a29da3e03063e8d37c717f35b68341e4957f07b620b48627be1c51ad4`.
Resource report: `dist/init-v2-independent-final-resource.YKMj7i/report.json`.
These receipts authorize normal DEVELOPMENT main commit/push only. This minimal
documentation receipt changes holistic source identity: e742 artifacts stay
immutable prior-input evidence, never relabeled as the delivery commit. Fresh
exact-commit ALL FOUR CI families are PENDING until the coordinator verifies Go,
packaging, compatibility+aggregate and resource+aggregate. No tag, public release,
PR/branch-protection change or native-service execution is authorized/inferred.

### Native/live boundary (not wizard service proof)

The user's existing production app login is generation1 and valid; init must
retain it without fresh login/refresh/remote validation. User-reported native Mac
both-model CLI text works. Luna text/tool-call plus manual full-history tool-result
continuation passed. Separately, retained local Sol testing completed3 SSE requests
with tool-result correlation but FAILED the UI wait; Luna/insertion were NOT
reached there. Do not conflate these with synthetic CLI20/oracle14 acceptance or
with full production lifecycle/native user-service proof. No such live calls or
real credential reads occurred in this implementation/delivery.

Native macOS LaunchAgent and Linux user-systemd execution are STILL PENDING.
After exact-delivery packaging CI is green, the coordinator may hand a fresh Mac
artifact to the volunteered user for explicit init/LaunchAgent consent and private
backup tests using docs/init.md. Cross-build/synthetic success does not clear that
gate. Installer stays noninteractive and never runs setup automatically.

## Narrow v258 native-contract follow-up (historical review checkpoint)

Auth/TUI and FS/config re-reviews CLEARED source
`40255e42e8eba52a6ef0dd68fc8216e809b5fa606323a27570c58915e960bd72`
(256 files); independent fast checks/build/race20/plain CGO0 targets/PTY smoke
also passed. Service review BLOCKED the normal native systemctl contract:
EnvironmentFiles and unloaded ExecStart empty struct arrays produce NO line,
and executable/loaded-argv/runtime dollar forms are distinct. Prior402 receipts
and `dist/init-fix-diags/READY.md`/`source-receipt.json` remain immutable; they do
not establish acceptance of this follow-up.

Narrow changes now awaiting focused service re-review:

- Normalize ONLY omitted EnvironmentFiles, plus omitted ExecStart ONLY when
  LoadState=not-found, after a completely successful typed response. All other
  requested fields remain required; unknown fields, loaded empty/missing command,
  nonempty environment files, missing scalars, errors/cancel/overflow/incomplete
  transport and truncated final lines refuse. `--all` is not used as a fake fix.
- Linux marker v3 uses `@` to separate a literal executable token from explicitly
  supplied argv[0]. Loaded argv retains dollar pairs; runtime/proc argv contains
  literal single dollars. Compare actual v258 raw path/joined loaded argv against
  the exact owned template, and independently verify runtime argv/UID/executable.
  Environment uses its distinct shell-quoted string-array format. No new command,
  property allowlist, SDK/module, timeout or capacity is introduced. macOS format,
  auth/stop reconciliation and FS transaction behavior remain unchanged.
- Positive source-derived fixtures omit empty specialized-array lines and use
  RAW ExecStart path/joined LOADED argv rather than invented quotes/pre-expansion.
  Tests cover normal absence, fresh installation, loaded/idempotent service,
  omissions of every other property, unknown/incomplete replies, dollar triples,
  literal executable spaces/percent/dollars, config quotes/backslashes and
  independent runtime argv checks. A loader-source audit also pins
  string_is_safe: executable ASCII quotes/backslashes reject BEFORE any file/
  supervisor access rather than using an impossible successful loader fixture.
  [Exact official v258 references/hashes](../provenance/systemd-init.md).
- Nonblocking FS regression improvements only: both invalid BOM fixtures now put
  a comment before [ai] so the UTF8 guard is necessary; actual new-config rollback
  remove.dirsync failure must retain ErrRollback and block prior-service restore.
  Documentation/source comments state the compare guards are best-effort, NOT
  atomic filesystem CAS; same-UID hostile/root/nonlocal filesystem exclusions
  remain. No cleared auth or FS production logic was changed.

`v2-contract-first.log` preserves the meaningful pre-fix failures after fixtures
were corrected to native format: normal uninstalled service rejected, executable/
argument dollar forms conflated, path cases rejected, Environment dollar decoding
failed. No assertion was suppressed. `v2-contract-second.log` passed after the
production fix; `v2-focused-first.log` passed setup/credentials/command tests.
`v2-make-check-first.log` then preserved a nilness failure: the obsolete later
commandErr check was provably redundant after the new initial complete-success
guard. Removed that duplicate only, without weakening the initial guard or
suppressing analysis. Contradictory not-found command/env-file data also refuses.
`v2-executable-path-first.log` preserved the source-audit regression: quotes and
backslashes incorrectly reached the mocked supervisor despite the native loader's
string_is_safe rejection. Added the preflight guard and changed positive path
fixtures to keep those characters in config arguments, not executable paths.
The earlier second-check receipt precedes that source-audit guard and is retained,
not relabeled. FINAL local receipts after the guard: `v2-make-check-third.log`
PASS (full check, unchanged four5s fuzz budgets,36 fast packaging fixtures and10
fast resource fixtures), `v2-build-final.log` PASS, BOTH plain CGO0 targets PASS,
`v2-race20-final.log` PASS for setup/credentials/command packages, fmt/diff PASS.
`v2-shipping-init-final.log` PASS uses the NEW v2-final plain Linux binary:
detached no-TTY/no-state and real private PTY missing-login No/lock-only assertions.
Final plain builds/logs use v2-final paths; prior402 and intermediate v2 artifacts/
markers are untouched. This is not native service, fresh two-path packaging/32
mutations/CLI-oracle hard-gate/full-resource acceptance.
Final source identity and narrow review handoff are in
`dist/init-fix-diags/READYv2.md`. NO new full package/resource freeze until focused
service clearance.
No host supervisor/user files/provider access, commit/push/tag/publication.

## Source402 correction record (historical; service review subsequently blocked)

ALL THREE initial reviews blocked source
`5e49a108a050a85e3013bad5d4c6024bf86d8b59072e2eef8c2631b42b7ef1ac`.
The independent verifier nevertheless passed that exact frozen247-file source:
checks/race20/CLI20/oracle14/both-target two-path package36+32/shipping init smoke/
full resource446 generations0 catalogs. Those passes do NOT clear the concrete
review defects. `dist/init-review-v2-20261017`, its manifest, resource report and
`dist/init-diags/READY.md` remain immutable prior-source evidence; no metadata is
relabeled. The corrected source below is NOT that package and has no new package
or full-resource acceptance receipt yet. No commit/push/live-service/provider
execution is authorized.

Implemented correction design (not independent clearance):

1. `WriteOutcome` separates applied/durable state and captures staged inode/content
   BEFORE rename. Post-rename destination-read/sync errors retain that identity
   and a typed commit-uncertain error. Shared compare-guarded rollback handles
   service journals AND config, including newly created targets and failed-but-
   applied writes; rollback uncertainty preserves primary failure and blocks
   automatic service restoration. Detected replacements refuse through best-effort
   inode/content guards; check-plus-rename/unlink is NOT atomic CAS. Same-UID
   hostile/root and nonlocal filesystem races remain outside the boundary.
2. Stop reconciliation is armed BEFORE dispatch. A command may mutate then error/
   cancel; independent cleanup reobserves only a previously verified managed
   service after auth Close. A deregistered but still-live prior PID blocks
   bootstrap (nonterminating signal-zero check, no name discovery/unrelated kill).
3. Closed typed command results retain bounded separate stdout/stderr/status,
   startup/cancel/overflow information and redact formatting/JSON. The COMBINED
   cap remains16KiB; supervisor budgets15s, active-check5s and terminal poll100ms
   are unchanged. Generic launchctl failure is NEVER absence. Target-specific
   definitive not-found is distinct from foreign/indeterminate; failed bootstrap
   does not confer ownership or authorize foreign bootout.
4. Linux effective-state inspection checks ExecStart/argv/environment/type,
   drop-ins, NeedDaemonReload, environment-file/pass/unset overrides and real
   positive MainPID/ExecMainPID. A bounded Linux process inspector verifies UID,
   argv, executable inode and stable start identity, never environ. Only strict
   unambiguous systemctl representations are accepted; unsupported display/path
   forms require foreground/manual setup. New effective state is rechecked after
   reload BEFORE enable/start. Generated unit marker isv2; old/modified units
   intentionally require manual migration. XDG config default HOME/.config is
   explicitly pinned in BOTH unit and supervisor environment. Environment dollar
   encoding is separate from ExecStart expansion encoding.
5. Atuin discovery is distinct from active-path evidence: automatic selection
   requires known18.19 and pinned-newer rules to agree on the sole EXISTING path;
   unknown/disagreeing layouts, ignored legacy/current/env candidates and creation
   require explicit --atuin-config. UTF-8 is checked BEFORE decoding (both UTF16
   BOMs refuse). Authorized scalar types are enforced, so aggregate comments are
   retained through refusal. NaN-aware semantic equality accepts unchanged NaNs.
6. Actual isolated PTY Answer tests cover EOF, hangup, cancellation DURING real
   poll, oversized input and the exact bound, with event synchronization rather
   than sleep budgets. All process/service/file tests use synthetic fixtures or
   private roots; process environ is deliberately unavailable in fixtures.

New correction logs: `dist/init-fix-diags/` (0700). `focused-first.log` preserved
an expected old resolver assertion (it still expected guessed creation); updated
that fixture to require explicit selection and added ignored-XDG/newer-only/
unknown-version controls. `focused-second.log` preserved a test-source missing
brace/parser failure; diagnosed and fixed without changing assertions.
`focused-third.log` passed the corrected suites. `make-check-first.log` then
failed SA4006 on an unused resolver-test result introduced while replacing the
old guessed-creation assertion; fixed only that unused binding, with no assertion
or lint suppression. The first focused race20 sweep also passed; final checks
and final source identity are recorded in the NEW
ignored handoff after execution, never inferred from old receipts. Corrected-code local receipts: `make-check-second.log` PASS (full checks,
unchanged four5s fuzz targets,36 fast packaging fixtures and10 fast resource
fixtures), `build.log` PASS, both CGO0 linux/amd64 and darwin/arm64 plain builds
PASS, `race20-final.log` PASS for setup/credentials/command packages, and full
fmt/diff checks PASS. `shipping-init-first.log` PASS uses the new plain Linux
binary with synthetic detached no-TTY/no-state and real private PTY missing-login
No/lock-only assertions. This is NOT service execution or a packaged-source
identity receipt. `validation-final.exit` and shipping exit markers are0.

No new package or seven-minute gate is started before critical re-review to avoid
freezing known-defective source again. No current-source CLI/oracle hard gate,
32 artifact mutations, two-path package or full-resource PASS is inferred from the
old5e49 receipts. Existing runtime/provider/catalog/resource limits and all fast/
default validation gates are unchanged.

## Initial implementation record (historical; not review clearance)

Base: clean `630ea5087fc1f9e6a17b963e55a29df9f50bf5cf` (coordinator reports all
four CI families green for that base). This patch is not committed or pushed.
No tag/release/publication is authorized. [Implemented behavior, limitations and
native manual acceptance](../init.md).

## Scope

- `cmd/atuin-ai-server/init.go`: canonical init dispatch/options, TTY-before-state,
  immutable bounded server-config resolution, ordinary runtime auth adapter,
  OS-account-home and native supervisor composition. Existing serve/auth/version
  APIs remain; public store-lock error text/normal command behavior is preserved.
- `internal/setup/setup.go`: default-No orchestration, lifecycle Close barrier,
  typed-lock managed-stop consent, safe restoration and partial completion.
- `internal/setup/terminal*.go`: bounded cancellable controlling-terminal input,
  no stdin approval channel, codes only on TTY; no fullscreen framework or SDK.
- `internal/setup/service.go`: stable self-copy/module/hash validation, exact
  generated-unit recognition, launchd/login and systemd USER definitions,
  minimal environment/fixed allowlisted exec, bounded active checks and rollback.
- `internal/setup/files.go`: descriptor-relative nofollow owned regular/single-link
  file access, safe directories, exclusive private backups, inode/content compare,
  atomic synced replacement/removal, mode preservation and noops.
- `internal/setup/atuin.go`: conservative explicit/env/existing/legacy resolution,
  bounded TOML statement/string scanner, three-key patch, semantic guard, exact
  model/other byte preservation and unsupported-layout/protocol refusal.
- `internal/credentials/{record,store_unix}.go`: distinguish genuine flock
  EWOULDBLOCK/EAGAIN from permission/corruption. Sentinel wraps ErrStore with the
  SAME error text. Existing commands map it to the prior public error result.
- Documentation: README, auth/ops/packaging, implementation/progress, `docs/init.md`
  and this checkpoint. Installer source/behavior and runtime provider/catalog/
  HTTP/model/resource limits are unchanged.

## Synthetic coverage and safety boundaries

All new tests use synthetic in-memory data or private test roots, never real user
HOME/config/auth/service files. No actual systemctl/launchctl/provider calls.
Coverage includes all auth/service/config consent combinations; no TTY; real
injected Linux PTY missing-login/decline; EOF/cancel; login/Close failures; real
Manager expired-refreshable preservation with a network-failing transport and
zero storage writes; typed lock vs permissions/corruption; recognized-service
stop/restore/refusal and secondary failure; foreign unit/arbitrary binary
refusal; generated XML/systemd path/percent/dollar/control handling; source
identity/startup negative controls; foreign loaded fragment/registered label
refusal; concurrent source/config/unit/binary startup-input refusal; copy noop; transactional old-byte/prior
activation restoration and rollback failure; bounded supervisor output/allowlist;
TOML unknown/missing/multiline model/comments/sections/no-newline/duplicates/
quoted/dotted/inline/Hub refusal; resolver macOS18.19/XDG/newer ambiguity;
filesystem links/permissions/unsafe ancestry/inode replacement, private backups,
mode preservation, no-overwrite backup and idempotence. Tests do not equate stub
success fields or generated files with actual service execution proof.

Pure-Go Darwin/arm64 and Linux/amd64 shipping targets remain required. Cross-build
is NOT native LaunchAgent proof. The user's prior both-model application evidence
is not repeated or inspected; setup needs no new provider/auth validation. No
proof/Pi/state imports, private E2E harness use, history/sync/profile modifications,
root services, sudo or automatic linger. Existing installer stays noninteractive.

## Validation and preserved first failures

Owned ignored diagnostics: `dist/init-diags/` (0700). Logs/output are synthetic
or public build/test metadata only. Completion/result markers are written only
AFTER each command exits; absence is not a pass. Final independent-review handoff
must report actual markers and the immutable package manifest/source inventory,
not reuse prior-source development artifacts.

1. Initial focused tests failed because two old tests expected exact ErrStore on
   genuine lock contention. Diagnosis: intended new internal sentinel, not a lock
   bypass. Diagnostic-store test now asserts ErrLocked; ordinary auth/serve map
   it back to the prior public ErrStore. This failure was observed before later
   focused receipts; no blind retry.
2. `focused-first.log` / `fs-first.trace`: new DiskFiles tests failed. Trace showed
   testing.TempDir's leaf was0775 under the host's umask002. The filesystem
   correctly refused writable ancestry. Test roots now explicitly chmod their
   OWN temporary root0700, matching existing credential-store fixtures; production
   permission checks were NOT weakened. `focused-second.log` passed.
3. `make-check-first.log` failed Staticcheck ST1005 on two new capitalized error
   messages. Fixed those error strings; no suppression/budget change. Subsequent
   full commands use separate logs; no failure is overwritten.
4. `make-check-second.log` failed two existing packaging fixtures: launching Make
   as an asynchronous shell job inherited ignored SIGINT, so the installer SIGINT
   trap negative control could not fire; concurrently, the unchanged real-CLI
   confinement entrypoint generated two nonignored Python bytecode files that
   the source-inventory gate correctly rejected. The first package attempt
   (`package-first.log`) also refused those same files before output creation.
   Diagnosis retained; only the two task-generated bytecode files were removed.
   Run full Make/check in the FOREGROUND and finish compatibility before source
   packaging. Neither installer nor verifier assertions were weakened.
5. `resource-first.log` stopped before the shipping workload: independent package
   verification correctly rejected a stale exact stdlib source-notice hash/size
   pin. Native setup adds debug/buildinfo, os/exec and os/user. Comparing old/new
   notice sections found55 additions, zero removals/changed old sections; reviewed
   Go BSD headers and FreeBSD ELF BSD-2-clause notice are fully retained by the
   existing producer. The consumer pin is now335056 bytes / SHA256
   `99df598e700d9967c194f5993b5b1095c0f007a0a15d2c2dc07e89fffce9b265`,
   with the exact24-entry legal set/other hashes/assertions unchanged.
   [Legal review](../provenance/dependencies.md). Initial BOTH-target/TWO-path
   package `dist/init-review-final-20261017` is immutable prior-source evidence,
   superseded by a new whole-source package after this correction.10 resource
   fixture +10 namespace/wire tests passed before this refusal; no full resource
   result was claimed. Original logs and public legal delta are retained.
6. `terminal-first.log` passed real detached-no-TTY/no-state and PTY synthetic
   missing-login/decline tests plus focused setup tests. Darwin CGO0 cross-build
   succeeded before the final whole-source packaging gate.

Review/acceptance commands (exact completion receipts live in ignored evidence
and the final handoff, so this source file does not claim unexecuted results):

```sh
make check
make build
git diff --check
go test -race ./internal/setup ./internal/credentials ./cmd/atuin-ai-server -count=20
ATUIN_E2E_MANIFEST=/home/pi/.cache/atuin-ai-phase8-fixed-cli/manifest.json \
ATUIN_ORACLE_MANIFEST=/home/pi/.cache/atuin-ai-phase8-fixed-oracle/manifest.json make compatibility
make package-dev VERSION=v0.0.0-dev.init OUT="$PWD/dist/init-review-v2-20261017"
python3 -B -I tests/packaging_test.py --artifacts "$PWD/dist/init-review-v2-20261017"
make resource ARTIFACTS="$PWD/dist/init-review-v2-20261017" OUT="$PWD/dist/init-resources-v2-20261017"
```

The package snapshots all tracked + nonignored untracked source, honestly dirty;
both targets are built in TWO fresh absolute paths and independently verified
with36 default tests/32 mutations. Source identity belongs to the manifest, not
this document's future self-reference. Any later source edit requires a fresh
package. The optional full resource receipt, if executed, must retain real60s/
300s defaults and exact446 generations/zero normal catalog calls; no shortened
budgets or stale shipping binary are acceptable. New init has no default network
or seven-minute test; Make/check resource gates remain unchanged.

The full foreground Make/check passed after those diagnosed environmental
failures; final package/source identity and final post-hardening race/check receipts
are to be read from the ignored final logs/handoff. Final self-review additionally
requires supervisor-loaded identity (not just local unit bytes), refuses unknown
prior enable states, rechecks all startup-input snapshots before activation and
reports uncertain stop/rollback failures rather than restarting on an unsafe
assumption. Foreground instructions work even if the stable binary/default config
has not been installed. Root init is refused before config/state access.

## Review boundaries / questions

Independent review is required for credential shutdown/managed-stop safety,
service transaction rollback and fsync/ancestor checks, strict supervisor context,
TOML scanner conservatism and installed-version path ambiguity. The implementation
intentionally refuses unknown/modified units rather than migrate them; unknown
layouts/version override ambiguity need explicit path/manual setup. Unsupported
GUI/user-manager environments use foreground instructions. Multi-file operations
are not one crash-atomic transaction; SIGKILL may leave owned staging files.
Same-UID hostile/root processes and network filesystems remain out of scope.
Neither code nor synthetic tests establish native Mac or Linux service execution,
upstream entitlement, provider lifecycle/replay completeness or release readiness.
No commit/push until independent review and separate authorization.
