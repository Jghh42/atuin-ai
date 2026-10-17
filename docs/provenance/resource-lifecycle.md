# Phase9b resource / lifecycle evidence provenance

These independently authored test programs are project Apache-2.0 material.
They reuse the already-reviewed repository production composition and test-only
Bubblewrap selector. Companion TCP partial-write witnessing adapts this project's
`internal/httpserver/native_cancel_test.go` / `native_cancel_trace_test.go`
method, not an upstream implementation. No new dependency, vendored provider,
Pi/Codex subprocess, production profiling endpoint or runtime flag is introduced.
[Checkpoint / first failures](../plans/phase9b-status.md).

## Independent local acceptance / source-identity boundary

BOTH focused measurement/isolation reviews CLEARED the corrected closed-eight-
input snapshot, constructor rollback, actual wire/confidentiality tests, CI
failure propagation and finite retention/sampling claims. Independent make-check/
build/diff,10+10 tests/helper+http race20, BOTH-target/TWO-path package, ENTIRE
resource FULL PASS and postrun36 tests/32 mutations/shipping smoke/current
consumer verification PASSED on source inventory
`08573867a21ea1d37fe03d43ef95e4fe6a3f3c7bfa093392afa810094beb0046`.
The immutable prior-input artifacts are `dist/phase9b-independent-final-package/`
and `dist/phase9b-independent-final-evidence/`; exact hashes and measurements are
in the [acceptance receipt](../plans/phase9b-status.md). Earlier unsafe5fb snapshots
remain superseded; the intentionally interrupted verifier RUNNING report is NOT
converted into a full PASS.

This documentation receipt changes holistic source identity. Never relabel085
artifacts as a later commit: exact-HEAD CI must build/measure its OWN source and
DEVELOPMENT artifact. Local independent acceptance does not imply hosted CI,
native Mac execution, production live/catalog or release/publication acceptance.
ALL FOUR exact-HEAD CI families remain PENDING for delivery; normal focused
commit/push only is authorized. No new production code, knob, dependency,
profiling endpoint or timeout change is part of this receipt.

## Reproduce (Linux amd64)

Requires exact Go1.27.2, Python3.9+ stdlib, OpenSSL, GNU `/usr/bin/time`, and
usable unprivileged Bubblewrap0.11.1 user/PID/mount/**network** namespaces.
Missing or restricted prerequisites **FAIL**, never green-skip:

```sh
make resource-test                    # fast invariant/mutation tests only
make resource-isolation-test          # fast REQUIRED namespace/wire/rollback gate
make package-dev VERSION=v0.0.0-dev.resource-local OUT="$PWD/dist/new-package"
make resource ARTIFACTS="$PWD/dist/new-package" OUT="$PWD/dist/new-evidence"
```

Both destinations must be fresh. The existing packager builds minimum Linux
amd64 and Darwin arm64 targets from TWO different absolute paths/caches, records
actual complete source/HEAD/diff identity, build settings/legal inventory and
binary hashes. `run.py` independently verifies its exact current-source assets
through `tests/packaging_contract.py` before measuring THAT raw Linux binary;
there is no custom resource-build variant. Source changes require a fresh
package; never edit a manifest or relabel an earlier snapshot.

The explicit `--case` diagnostic selector always produces **PARTIAL**, even if
its cases pass. Full `make resource` includes real60s inactivity +300s generation
cases, so takes about7minutes. Do not put that duration in every Go unit test.
The separate `Offline shipping resource gates` PR/push/manual CI job provisions
checksum-pinned Bubblewrap in a private prefix, uses Phase8's exact-path hosted-
job-only AppArmor userns setup when needed, always removes that profile, and
uploads only development package + numeric reports. Its always-running
`Phase9b resource validation aggregate` rejects failed/cancelled/skipped resource
jobs; the resource job additionally requires a present COMPLETE PASS receipt,
exact counts and successful cleanup. Delivery/release MUST independently require
ALL FOUR exact-HEAD workflows/check families: Go checks, Packaging consistency,
Phase8 validation aggregate AND Phase9b resource validation aggregate. Compatibility
aggregate alone is NOT overall clearance. Required-check configuration is
EXTERNAL and is not changed/claimed here. Local selection defaults
to reviewed SYSTEM `/usr/bin/bwrap`; CI may set the independently verified
`ATUIN_TEST_BWRAP_MANIFEST` selector. No developer sysctl/policy/root install.
Branch protection/release policy is external; this code does not configure it.

## Isolation / synthetic state

The measured executable is unchanged. Its child-only environment is allowlisted:
PATH, private HOME/XDG_CONFIG_HOME/XDG_STATE_HOME/TMPDIR, HTTPS_PROXY pointing at
loopback CONNECT, SSL_CERT_FILE pointing at a private ephemeral CA,
SSL_CERT_DIR pointing at an EMPTY private directory, GOTRACEBACK=none. GOGC,
GOMEMLIMIT, GOFLAGS, GODEBUG, HTTP_PROXY, ALL_PROXY, NO_PROXY and operator
credential variables are not inherited. Standard TLS verification and fixed
production URLs remain intact. No real provider DNS/dial is ever needed.

`tls_fixture.py` permits CONNECT ONLY to `auth.openai.com:443` and
`chatgpt.com:443`; correct SAN certificates and SNI are mandatory. Auth routes
are deny-default during serving (fresh fake expiry lasts beyond the suite).
Only exact catalog GET/path/client_version and Responses POST/path are allowed;
Host/method/query, synthetic Bearer/account pair, originator/beta/content headers,
store=false/stream=true, model, complete decoded history, per-turn context,
exact golden policy and both tool schemas are checked. No redirect, remote
resolution, remote dial, forwarded tunnel, fallback or captured request log.
History is discarded immediately after validation; only numeric witnesses remain.

Proxy correctness is NOT the no-egress boundary. Bubblewrap supplies a distinct
network namespace containing ALL server/fixture/load/sampling processes. Driver
checks `/proc/net/dev` is only `lo`, route table is empty, a TEST-NET remote
connect fails, and loopback works. A fresh tmpfs root has read-only `/usr`,
individual verified binary/helper mounts, a PRIVATE MINIMAL SOURCE SNAPSHOT,
new `/proc` and `/dev`, and exact writable scratch/evidence mounts. The whole
working repository is NEVER mounted: read-only protects integrity, not secrecy.
The outer independent package consumer still checks FULL actual source/HEAD
truth. `source_snapshot` then copies only the eight closed required public Python/
golden fixture paths, checking each against THAT verified package's actual source
inventory (content, size, normalized mode, regular single-link type and non-symlink
ancestry). It checks the exact subset tree before/after execution; the inner
no-Git driver rechecks it too. No `.git`, ignored directories, credentials,
private probe, config material or host source ancestors are exposed. The private
snapshot is mounted ONLY read-only at `/repo`, never under writable scratch or
evidence, and removed afterward. Its selected file inventory and original package
source-inventory digest are recorded, not misrepresented as a second Git checkout.

`namespace_test.py` uses the SAME mount builder with an OWN synthetic checkout:
synthetic `.git` and Git-ignored private/probe/config sentinels must be absent/
unreadable inside, while required public files and executable stand-ins remain
available. It never installs sentinels in or reads real `.git`/private material.
Source write-open must fail EROFS. Actual loopback CONNECT/TLS wire tests cover
positive verified-CA catalog, wrong authority/port, duplicate headers, wrong or
missing SNI, inner Host and bad framing, with all DNS/outbound connection helpers
deny-guarded. All run inside the mandatory no-egress namespace, not host networking.
Fresh-root ownership preserves the secure store's UNCHANGED ancestry policy;
no host HOME, global CA, privileged host mount, broad permission bypass or host
namespace policy change is used.

Private CA keys/certs and fake JWT/account metadata are generated at runtime
under0700 scratch (keys0600), never committed/uploaded. Short-lived setup helper
calls the reviewed Manager.Login using a deny-default synthetic RoundTripper
with exact device/exchange payload checks; no network transport exists there.
It closes the real durable secure store before shipping `serve` acquires the
lifetime lock. Every scenario has fresh private HOME/XDG/state/temp, except the
intentional SAME-store restart lock witness. No real token/store is imported.

## Workloads and barriers

Fresh shipping processes isolate warmed-idle, four-normal and four-near HWM.
Each first warms native login-record load, local bundled catalog, real TLS chat parsing and
64KiB output. No shipping forced GC or race instrumentation.

- Normal:16384 decoded history characters,65536 output bytes per request.
- Near:exactly4193280 wire bytes (4MiB-1KiB), roughly699k decoded meaningful
  history characters, mostly JSON `\u0061` escapes. Valid JSON/transcript,
  below180000 estimated tokens; not whitespace, malformed/rejected padding or
  a purely synthetic provider helper. Distinct histories for all four requests.
- Four requests stop ONE BYTE short of their bodies. Owned server socket RX
  and driver TX queue consumption witnesses precede a fifth request's503/busy,
  proving four predecode admission slots without queueing or an early rejection.
  Release last bytes, require four simultaneous native upstream generations,
  exact semantic full-history projection and exactly one generation per request.
- Fixture next sends bounded call argument events and text. All four clients
  observe their projected text after preceding argument events, while fixture
  still holds all four completion snapshots. Only then release completions.
  The100ms dwell is solely for20ms sampling AFTER observable barriers, never a
  substitute for joins/dispatch/cancellation evidence.
- Near completed event is1047552 bytes (1MiB-1KiB),32 schema-valid write_file
  calls, one argument about254KiB and31 about15KiB, plus~300KiB text, fitting the
  ACTUAL one-event final snapshot budget. No local tools are executed. Counts,
  per-call maximum, frame/projected/request/provider bytes are reported.
- Separate hard rejects:valid4194305-byte request413/no generation; over1MiB SSE
  event, over256KiB arguments and33calls produce exactly one terminal error,
  no done/executable calls/retry. These are not near-limit successes.

## Lifetime / cancellation / retention

Shipping process:5+5 warmups, then100 successful completions AND100 actual client
TCP disconnects, plus one finite16MiB-output nonreading client. Companion process
independently repeats the same counts. Every request has exactly one generation;
all fixture handlers, samplers and child processes are joined. Ownership is
registered IMMEDIATELY after spawn, before PID discovery/descriptor wrapping/
sampler start. Constructor rollback kills/reaps ONLY that owned process group,
joins any started sampler and closes every tracked raw descriptor/stream on all
later exceptions. Namespace regressions inject post-spawn discovery, pre/post
sampler start, actual Thread.start, spawn and second-pipe failures; assert group/
FD/sampler cleanup and an unrelated process remains alive. Fresh-state file
metadata must stay unchanged (no transcripts), owned FD/socket counts cannot grow.

Disconnect cleanup is bounded to1s from the OBSERVED client socket close to
external upstream EOF. Shipping nonreading evidence bounds default backpressure
completion around5s but does NOT invent an observed server write error. The
companion's test-only TCP listener bounds send buffering and records actual
partial `net.Conn.Write` timeout start/end/deadline/requested/written bytes.
Inherited-pipe witness confirms DEFAULT five-second deadline and upstream close
within1s from that observed error. Realtime nanoseconds are same-host; monotonic
elapsed clocks separately bound the scenario. Existing accelerated real-TCP unit
cancellation tests remain additional evidence, not default-duration substitutes.

Real shipping timeout cases:HTTPS response HEADERS OPEN/body idle until60-second
inactivity; comment heartbeat every1s until300-second generation deadline. Each
must yield one generic terminal error, no done/retry, upstream close and a joined
handler. Both signals separately cover authenticated idle, empty idle, owned-
socket-consumed empty partial body, and four active generations; listener must
refuse admission within1s, process exit/drain <=6s and a new process reacquire
the same lifetime store lock. No unrelated host process is signaled.

Companion alone uses runtime.NumGoroutine, runtime/metrics and MemStats after
matched forced-GC snapshots at warmed baseline and EVERY10 paired cycles through
100. Goroutine allowance2, heap512KiB/512objects, FD/socket allowance0 are fixed
small jitter/cache bounds, independent of the256MiB RSS gate. End-to-baseline and
sustained monotone-tail growth beyond these bounds fail. Raw11-snapshot sequences
are retained; nonzero bounded heap changes are explicitly visible, not claimed
as zero-byte retention or proof of a plateau. The prior +399-object sequence trends
up despite dips; it does NOT establish that growth eventually stops. Exact
allowance/+1 and a failure caused SOLELY by monotone-tail growth (while endpoint
is below baseline) are regression-tested without changing any allowances. This finite test cannot exclude tiny leaks below the
stated resolution, unsampled transient peaks, longer-lived caches or different
hardware/load behavior. Independent review should assess those limits rather
than treating a PASS as a mathematical leak proof. Shipping has external
FD/socket/thread snapshots only—NEVER companion heap/goroutine numbers.

## Measurement / output

`run.py` observes GNU time's single server child PID, samples ONLY that PID's
`/proc/PID/status` VmRSS/VmHWM/Threads and owned fd/socket inode counts with a
REQUESTED20ms period, NOT a guaranteed interval. Each process report records
`max_observed_sample_interval_ms` from actual sample timestamps (e.g. prior near
sampling had~53.5ms gaps); fixed20ms wording must not imply those gaps were absent.
It does not dump other processes, socket addresses or request bodies. GNU
`time -v` supplies independent final kernel maxRSS of the post-exec server child.
Outer wrapper wait4 usage is recorded as **NOT_SERVER**, because direct Python
spawn can inherit the driver's pre-exec high-water mark. Linux zombie FD EACCES
is classified only after a bounded independently observed exit; any live access
failure, sampler/fixture background error or failed join FAILS.

Hard warmed-idle HWM <=64MiB; hard four-near <=256MiB. Soft idle <=32MiB and four-
normal <=128MiB are reported separately. Conservative max of sampled VmHWM and
GNU time kernel maxRSS determines gates. GOMEMLIMIT remains UNSET; it is a GC
target, not RSS enforcement. There is no tuned repeat replacing default gates.

Report includes kernel/CPU/affinity/page size/physical RAM, own cgroup ancestor
limits, rlimits, Go pin, package build flags/environment, binary/helper/Bubblewrap
hashes, source inventory, numeric requests/concurrency/bytes, all hard/soft
outcomes, cancellation/timeout/signal/restart/retention sequences and cleanup.
Full suite must have exactly446 generations +0 automatic remote catalogs, peak4 concurrency and
zero active fixture handlers after cleanup. The bundled policy removes the old
15 metadata GETs (one per fresh process/cache lifetime), without changing any
446-generation workload, barrier, timeout or budget. Explicit positive/negative
TLS catalog wire tests remain intact and separate from automatic runtime traffic.
Reports are flushed after each case;
first failures are preserved, not overwritten by reruns or filtered to green.

All outputs are DEVELOPMENT/local same-host consistency, not independent
attestation. Final exact source/package hashes belong in generated manifests
and the external review handoff, avoiding a self-referential source inventory.
CA/fake state/scratch and owned child processes disappear before return. Only
safe numeric evidence and legally complete development assets may be uploaded.
Independent review and local full085 verification are CLEARED/PASSED, within
the stated finite resolution. Native Mac execution, real-account/provider/live
eligible discovery, ALL FOUR exact-HEAD hosted CI families and explicit
publication approval remain separate release gates.
