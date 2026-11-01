# Bounded private reasoning continuation (development candidate)

This candidate enables durable private continuation in the normal native runtime.
It is not a release, native Mac execution, live inference/account entitlement, or
universal model/private-Codex compatibility claim. Final independent full-feature
code review and delivery gates remain separate. [ADR0005](decisions/0005-durable-provider-continuation.md)
and [exact public schema/provenance](provenance/reasoning-continuation.md) define
the supported finite contract. No SDK runtime or copied SDK implementation is used.

## Unchanged public behavior

One chat request makes one native generation: `store:false`, `stream:true`, no
`previous_response_id`, retries, hidden model fallback or server-executed tools.
The CLI owns its transcript, permissions and local tool execution. Public JSON/SSE
fields and single-line framing are unchanged. Text may stream before completion;
all executable calls remain held until complete output/usage and the entire
capability/schema/safety-validated batch are accepted. `suggest_command` remains
terminal/nonexecuting and replays as assistant text, never a fabricated result.

Legacy tool IDs remain exact backend call IDs. New reasoning-dependent executable
exchanges, and later executable descendants of a retained protected exchange,
receive opaque **90-byte** `aic1_...` IDs. Clients must retain these exact strings
in tool uses/results; they are correlation handles, not paths, response IDs or
provider CallIDs. Every call and result in the issued whole batch must occur
exactly once, in the supported message layout. Parallel result completion order
is preserved. Removing a sibling call AND its result still fails authentication.
Every retained protected handle is verified, not only the latest one.

The prepared transaction reconstructs the entire original output once, including
original item IDs, backend CallIDs, exact raw arguments, supported reasoning and
ordinary text parts. Actual results follow in their actual incoming order with
original backend IDs. Pinned Rust removes leading whitespace only; original
leading bytes are recovered privately and trailing bytes remain exact. Protected
arguments recursively require canonical decimal signed64/unsigned64 integers;
fractions, exponent spellings, `-0` and overflow reject before tools. Legacy
number handling is unchanged.

Raw library `Stream`, `Open` and `Project` are intentionally not a bypass: raw
reasoning/protected histories fail closed. Only the prepared composition may
publish protected executable state.

### Supported replay metadata and bounded checkpoints

Protected assistant messages preserve required completed status and empty
annotations on every output_text part, plus optional nullable commentary/final_answer
phase. Missing required or unsupported/nonempty metadata rejects BEFORE tools;
ordinary legacy acceptance is not silently broadened. Optional logprobs support
only omission/null/empty array (null omitted on input). Function caller permits
only omission/null/exact direct; async only omission/null/false (input null omitted).
Program origin/async true/malformed variants reject in every phase, even without
reasoning/ancestry, never become server/CLI execution. ADDED is provisional;
DONE presence/metadata is authoritative and cannot be erased by final omission.

Reasoning DONE checkpoints keep only fixed digests/IDs, not one large raw payload
per item. All final replay bytes come from the bounded completed frame;128 separate
large DONE frames followed by EOF/cancellation cannot accumulate128 encrypted blobs.
Bounds precede hidden summary/content index arithmetic, including empty paths.
Ordinary slots also retain ONLY bounded per-kind data: message ID/kind plus fixed
identity/checkpoint digests, never inapplicable name/call_id/arguments/content/raw
metadata fields. Function-only names/IDs/arguments remain under existing32call/
64name/256ID/256KiB argument limits. Raw legacy comparisons are preserved through
domain-separated length-delimited identity/retransmission digests. Payload digests
exclude text/content BYTES but authenticate their required presence/TYPE; missing
or null DONE cannot become present-empty at final. ALL authoritative validation
failures accumulate, and all metadata failure flags are monotonic ORs; provisional
missing fields remain distinct from supplied malformed fields. In particular,
supplied null message status/content/role, output-text text/annotations and
function arguments are invalid nonnullable fields and stay failed from ADDED;
permitted provisional omissions may still complete. This field-specific rule does
not reject approved nullable phase/logprobs, function status/caller/async or
reasoning status/content/encrypted content (subject to executable-state checks).

## Local storage and lifecycle

The fixed `continuation-v1` child lives inside the existing application credential
store (0700 directory, regular single-link0600 files, descriptor-relative nofollow
operations). Credential JSON schema/codecs and saved login/config are unchanged.
Private snapshots use AES-256-GCM. A separately stored local **plaintext root key**
derives distinct sealing/binding/handle/ownership keys. This protects detached
blobs and accidental representations, not same-UID/root compromise, a backup
containing the key, arbitrary filesystem rollback, or forensic secure erasure.
Protect this namespace like credentials; never share it for diagnosis.

Snapshots retain only required provider items/reasoning/IDs/raw arguments,
discarded leading text and authenticated public-history/span references. They do
not archive user messages, results, contexts, instructions, tokens or raw SSE.
Private record/envelope layout2 explicitly rejects old unreleased layout1;
there is no guessed migration. Key.bundle and credential JSON formats remain
unchanged; projection/policy version advances. Each latest snapshot contains its own complete ancestry; there is no mutable
session-latest pointer, plaintext whole-store cache or background retention worker.

Hard limits are 2MiB **sealed including envelope**, 128 segments/1024 items/
4096 parts+spans, 1MiB discarded leading text, 128 records, 64MiB actual total
(including key/pending/crash residue), 132 scanned entries and four scope leases.
Absolute lifetime is at most24h and inherits the oldest segment expiry; access or
copy never renews it. Clock rollback over one minute fails closed. TTL is an
authorization bound; stopped-runtime files are not erased by a timer.

Persistence completes file fsync, close, rename and final directory fsync before
any protected tool. Durable bytes alone are not emission authority: original
request/scope/manager-lifetime and expiry checks still guard every tool/status/
done callback. Cancellation during final sync can leave a bounded orphan with
**zero** released tools. A disconnect after a partial network write is not an
exactly-once delivery guarantee and never causes automatic retry.

Every unusable credential publication suspends/revokes and joins native stream
cleanup, even if durable Begin fails. Failed Begin sends no OAuth and deletes no
key. Proven durable login/logout intent permits safe purge; logout still attempts
its credential tombstone after a purge failure and reports failure honestly.
Successful same-epoch token refresh preserves durable continuation. Normal Close
preserves snapshots/key for restart and finishes before releasing the credential
flock. Auth status and diagnosis never acquire/create/inspect/prune the child;
init refusal/no-TTY does not create it. Existing local login is not revalidated.

New private continuation objects use unexported opaque payload pointers and owned
mutable copies. Formatting methods alone are not a confidentiality boundary:
invalid Go `%w` can bypass them recursively. Tests cover nil/zero/value/pointer,
containers, encoded bytes, JSON and both slog handlers. Do not print private
credentials or arbitrary contexts; legacy credential presentation is not a new
opaque-object guarantee.

## Finite-window failure and attended recovery

Malformed/missing/lost/expired/corrupt state, wrong account/AuthEpoch/model/session,
changed stable policy/tool definitions/capabilities, or changed retained prefix
fails closed **before** a generation POST for a marked history. There is no
legacy fallback. The pinned client windows at **200000 serialized UTF8 bytes**,
retaining one first turn plus whole-turn tail. Windowing can invalidate strict
full-prefix binding before TTL/storage limits. A new user message inside the same
transcript does not remove protected ancestry. Start a genuinely NEW chat (`/new`)
for transcript/policy/window loss; do not edit IDs, remove selected exchanges,
ignore the window marker or switch models as an alleged repair. If ALL protected
exchanges are omitted, the unchanged wire cannot distinguish that from a legacy
transcript; there is no universal truncation-detection claim.

`/new` does **not** repair a globally missing/corrupt/wrong-owner key with existing
records. Stop serve and use explicitly attended **new-binary** logout/login with
consent, or separately attended strictly owned cleanup when necessary. Do not
request private files/journals, edit credentials, unlink the lock, automatically
reauthenticate, or promise recovery of lost state. Rollback to stable v0.1.0 uses
the SAME saved login/config but a **NEW conversation**; that binary cannot replay
protected IDs and its logout does not purge this new namespace. No public stable
asset or installer is modified by this candidate.

## Closed diagnostics and evidence

The existing seven log keys remain `time`, `level`, `msg`, `phase`, `category`,
`upstream_http_status`, `elapsed_ms`. Local continuation failures have phase
`continuation` and null HTTP status; no raw error/type/status prefix, ID, path,
model, argument, history, usage or private state is logged. At most one
`generation_failed` accompanies an attempted generic terminal. Only successful
done/flush plus complete native/lease/transaction cleanup permits optional
`generation_reasoning_handled` or `generation_reasoning_replayed`, categorized as
`reasoning_handled`, `reasoning_replayed`, or `reasoning_replayed_and_handled`.
Ordinary successes do not log. Elapsed starts before Prepare and ends at terminal
decision, excluding logger latency. Synchronous trusted stderr can block and
retain a handler/admission slot after upstream cleanup; no detached log queue is
introduced. Success diagnostics prove local handling/replay only, not entitlement.

The original CLI20/oracle14 suites remain separate and unchanged. A new pinned
CLI proof checks two actual server-process restarts, exact whole-output/result
replay, opaque IDs, uint64/int64 JSON and leading-only trim. Separate confined
shipping/companion stress checks failed/handled/replayed cycles, subsequent
success, four near-limit scopes and unchanged memory/FD/heap/goroutine budgets.
The corrected stress sampler fails closed on every read/thread/zero-coverage/join
failure and requires successful per-PID baseline/final VmRSS+VmHWM observations;
periodic20ms requests are not a guarantee of gap-free sampling. Four extra
concurrent128×900KiB reasoning-DONE sequences PLUS four concurrent128-frame
ordinary-DONE streams (300KiB name+300KiB call_id+256KiB inapplicable arguments
per frame) terminate without completed/tools and obey
the SAME256MiB memory ceiling; companion post-GC checks remain separate. Earlier
fb9 passing stress receipts predate the corrected gate and are historical, not
corrected-code clearance or evidence that a sampler failure actually happened.
These synthetic receipts do not establish the cause of an earlier real-machine
failure, private backend extensions, unknown installed clients, or live success.
