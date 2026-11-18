# ADR 0005: Bounded durable private provider continuation

- Status: accepted design; implemented development candidate, final full-feature review pending
- Date: 2026-11-01
- Supersedes: ADR 0002's no-hidden-state and ID=backend-call-ID rules ONLY for new protected executable exchanges

## Context

The user authorized the full bounded private-state solution. Independent V3
security (`d59fb0c8`) and protocol (`b93ec837`) design reviews cleared it.
These are design dispositions, not code or live compatibility approval.
Atuin owns the public transcript and local tools. Reasoning returned alongside
tool calls can require provider items that its public protocol cannot preserve.
A successful first text response does not establish tool continuation support.

## Decision

Keep `store:false`, `stream:true`, one generation per request, no retry, no
`previous_response_id`, no provider-session reinterpretation of Atuin SessionID,
no SDK runtime, no hidden model/request fallback and no server-side tool loop.
Public SSE schemas/framing remain unchanged. Legacy unprotected IDs remain exact.
The development candidate enables strict reasoning acceptance and prepared runtime
composition; this does not itself authorize release or establish live compatibility.

### Private records and binding

Use immutable self-contained snapshots under a fixed `continuation-v1` child of
the descriptor-anchored exclusive credential directory. Never persist a raw
request, user prompt, result, context, token, transcript or SSE archive. Retain
only required provider reasoning/items, original item/call IDs, exact arguments,
discarded leading text, item/part span recipes and keyed public-history references.
No dependency graph, mutable session-latest pointer or whole-store plaintext cache.

Private values contain only unexported opaque payload pointers, with no public
payload fields. Constructors clone mutable bytes/slices and nested ownership;
only deliberate trusted codec/accessor paths read payloads. Formatter methods
alone are insufficient: Go 1.27 invalid `%w` diagnostics bypass them recursively.
Adversarial tests therefore cover values/pointers/nil/zero and nested containers,
ordinary/invalid verbs, JSON and both slog handlers, including raw-byte decimal/
hex/base64 encodings and private identities/custom value contexts. Unsupported
formatting may show Go's type/pointer diagnostics, never payload data. Pointer
LogValue methods are explicitly nil-safe (no panic/stack-path fallback).

Protect snapshots with AES-256-GCM, fresh random 96-bit nonces and authenticated
schema/key/snapshot/scope/creation/expiry metadata repeated in the payload.
Derive separate AES, binding, handle, ownership and bundle-integrity HMAC-SHA256
subkeys. Separate `key.bundle` holds a random 256-bit root key, random 64-bit key
ID, account/provider/AuthEpoch commitment and durable clock high-water. Credential
JSON schema/version/fields/codec remain unchanged and stable-v0.1.0-readable.
The key is private local PLAINTEXT: this protects detached blobs/accidental
exposure, NOT same-UID/root compromise, complete directory backups or arbitrary
filesystem rollback. Owned-memory clearing is best-effort, not secure erasure.

New protected executable IDs are exactly 90 ASCII bytes:
`aic1_<16 lowerhex keyid>_<32 lowerhex random snapshot>_<2 lowerhex ordinal>_<32 lowerhex MAC>`.
Use a 128-bit truncated HMAC binding all these fields and the scope; compare in
constant time. Every present reserved marker must authenticate and belong to the
ENTIRE sealed ancestry. Malformed, missing, expired or wrong-state markers fail
before a generation POST, never fall back to legacy. Provider literal reserved
CallIDs are forbidden. The unchanged wire cannot detect removal of ALL markers.

Bind provider/account/AuthEpoch/model/Atuin session/projection version plus the
actual rendered stable instructions and exact ordered tool definitions/settings.
Also bind normalized capabilities/run preference, optional OS/distro/shell/
language (including presence), ordered user contexts/skills/overflow and explicit
prompt/registry/projection versions, even currently inactive inputs. Exclude ONLY
volatile TurnContext UTC date/PWD/LastCommand, invocation, token and credential
rotation generation. Ordinary token rotation preserves AuthEpoch and stored data.

Canonical history is a length-delimited message-aware grammar preserving every
user-message, block and empty boundary. Arguments sort object keys, preserve
arrays/decoded strings and exact numeric lexemes, never float64 approximations.
New protected executable arguments recursively allow ONLY canonical decimal
signed-int64/unsigned-uint64 integers: `0` or `-?[1-9][0-9]*`. Reject `-0`,
fractions, exponents and overflow anywhere (including unknown nested properties)
before persistence/tools. Legacy number acceptance is unchanged.

Store an issued contract for ALL K calls and results, K=1..32. Calls must appear
once in release order; results once after their own call, in any completion order.
The pinned assistant language permits text+maximal consecutive calls, separate
singleton user results, then further assistant call runs. Thus A/resultA/B/resultB
is valid; missing B AND its result is invalid even though the normalizer accepts
A/resultA. No global same-role merging or reset inferred from user-role plaintext.
Replay inserts the ENTIRE original provider output once at the first protected
segment, consumes public text/calls, then inserts original-call-ID outputs in
actual incoming result order. Later snapshots carry all ancestry themselves.

Keep raw provider text and pinned Rust `trim_start` client text distinct. Use the
explicit 25-codepoint White_Space set, never trim trailing bytes. Capture at most
1 MiB discarded leading UTF8 across delta/part boundaries and seal span lengths/
digests. Reconstruct original parts from that prefix plus matched client text.
Overflow/interleaving is a protected projection failure, not legacy normalization.

### Review corrections: finite ordinary metadata and checkpoint memory

Private unreleased record/envelope layout 2 (aic-record2/aic-sea2) rejects layout 1;
projection/policy versions advance without changing key.bundle/credential JSON or
public handles. Required completed message status/empty annotations, nullable
phase and supported empty logprobs are preserved by explicit codec metadata.
Function caller is only omitted/null/direct, async omitted/null/false; program,
async true, malformed/unknown execution origins fail at every snapshot, including
nonreasoning/raw calls. Known-null nonnullable input fields are omitted, never
invented. ADDED may progress; DONE metadata presence/digests cannot be erased by
completed omission/change. Unsupported annotations/metadata cannot be silently
lost during protected replay. Exact same-pin public schema facts are in provenance.

Reasoning DONE retains only bounded identity/digest checkpoints, never one large
encrypted/summary/content snapshot per slot. Completed replay data comes from the
single bounded final frame; four 128×900KiB-DONE streams followed by EOF remain
within the unchanged 256MiB production budget. All hidden indices are bounded
BEFORE arithmetic/lookup/empty-text fast paths. These are CODE corrections, not
new architecture or a weakening of shared 128part/frame/storage limits.
Ordinary slots likewise construct only bounded per-kind checkpoints, never retain
inapplicable message call strings/arguments/raw slices; legacy identity/ADDED
retransmission comparisons survive as domain-separated fixed digests. Required
text/content presence and TYPE are authenticated separately from excluded payload
bytes. All authoritative DONE failures accumulate; all metadata failure flags
are monotonic ORs, so missing later fields cannot clear an earlier invalid value.
Four additional ordinary 128-frame composite-DONE streams share the SAME budget.
These parser-only corrections do not change private layout 2/key/credential formats.

### Storage, resources and time

All directories are euid-owned exactly 0700; files are regular, single-link,
euid-owned exactly 0600. Use descriptor-relative nofollow/nonblocking opens and
fstat. Only fixed key, canonical random snapshot and owned pending filenames;
never a client/provider path. Unknown/unsafe entries are not deleted recursively.
Read-only diagnostics never acquire/create/prune the continuation child.

Hard ceilings: 2 MiB sealed snapshot INCLUDING envelope; 128 segments, 1024
provider items, 4096 parts and 4096 spans; 128 records; 64 MiB TOTAL actual bytes
including key/pending/crash residue; 132 scan entries; four admitted leases;
one synchronous pending writer, no background queues. Decode bounds precede
allocation; binary record grammar has fixed depth <8 and <=262144 work units.
Prune authenticated expired records then oldest creation/snapshot tie-break when
capacity requires it, one plaintext record at a time. No access-time LRU writes.
24h absolute expiry is MINIMUM inherited expiry, never renewed by access/copy.

Accept checked UTC millisecond years 2020..9999. Persist increasing high-water
BEFORE protected use; effective time is max(wall,persisted,in-process atomic floor).
Rollback >1minute fails closed. This cannot detect unobserved clock manipulation
or restored whole-filesystem backups. TTL is authorization, not background secure
erasure: stopped-runtime residues await bounded new-runtime maintenance.

### Durability and credential lifecycle

Validate complete provider output/usage, capabilities, batch, history/backend/
public collisions and projection BEFORE commit or first tool. Under the identity
operation gate, write/fsync/close/rename/FINAL directory fsync. A committed record
is NOT emission authority. After final sync recheck ORIGINAL generation, scope and
manager-life sources and EVERY inherited expiry; release store mutex, repeat
synchronous source/atomic-clock guards immediately before callback, with NO more
filesystem I/O. Cancellation/expiry during successful final sync retains a bounded
orphan and releases ZERO tools. Every protected tool/status/done uses original
publication context; parent-only context is for generic failure terminal only.
No exactly-once network-delivery promise, no retry and no post-done second terminal.
Kernel fsync/trusted sink stalls cannot be preempted by Go.

The context-aware channel operation gate has no waiter workers; diagnostic
TryLock remains fail-fast. Lock order: gate -> brief manager.mu (released) ->
store.mu -> FS. No manager.mu across callbacks/I/O/join; no store.mu across manager
calls/gate acquisition/join. Publication callbacks cannot reenter Manager.

Every unusable publication unconditionally memory-Suspend/revokes/cancels/joins
started private stream cleanup BEFORE return, including FAILED Begin on login,
logout or refresh. Join only native-stream completion (body/watchdog/callback),
not pending Publish or Prepared.Close waiting for the same operation gate.
Purge is separate, ONLY after proven durable Begin/startup intent, key unlink+
dirsync then safely owned state+dirsync, without requiring damaged-key parsing.
Failed Begin means NO purge/OAuth/durable-logout claim. Logout still attempts
credential logged_out Commit after successful Begin even if purge fails, and
returns fixed ErrStore. Successful refresh activates a fresh same-epoch scope
without rotating/deleting the key. Close cancels/joins, preserves committed files,
and closes dependent resources BEFORE releasing credential flock.

### Finite support and recovery

The pinned client automatically windows at **200000 serialized UTF8 BYTES**,
freezing one first turn plus whole-turn tail. Strict full-prefix binding can fail
BEFORE 24h/size limits when that window changes a retained protected prefix.
Changed policy/capability/model/session or edited retained history also fails
before POST. Require genuinely NEW chat, never ignore window markers, match an
arbitrary suffix or infer a safe reset from a new user message. No universal
all-turn/new-user/model-switch/windowed continuation promise.

A new chat does NOT repair a globally missing/corrupt/wrong-owner key with existing
records. Supported repair is explicitly ATTENDED new-binary logout/login at the
same saved config, or separately attended strictly owned namespace cleanup if
needed. No automatic reauthentication or request for private files/journals.
Rollback to stable v0.1.0 uses SAME login/config but a NEW conversation: stable
cannot interpret protected IDs and its logout does not purge the new namespace.

Known schema-validated reasoning+meaningful client-normalized text may terminate
without a text-session archive. One validated suggest_command remains terminal,
nonexecuting/safety-escalated, replayed as existing assistant text, not dangling
function_call or invented result. Earlier protected executable ancestry remains
required. Reasoning-only/whitespace-only without calls is not successful done.

Diagnostics use only closed labels, existing seven keys, no private errors/IDs/
paths/text/arguments. Optional handled/replayed success reports come only after
done and native/lease/transaction cleanup, unlock/cancel, at most once. Elapsed
starts before local prepare/generation and ends at successful terminal decision,
excluding logger latency. Trusted synchronous stderr may block. No ordinary
success report. See [schema/provenance](../provenance/reasoning-continuation.md).

## Alternatives rejected

Blanket reasoning-ignore, cache-only loss, transcript archive, SDK loop/retry,
session-as-provider-ID, partial batch replay, global role flattening, approximate
floats, contiguity assumptions, guessed private schemas, pre-fsync authority,
failed-Begin cancellation bypass and pretending /new repairs a global key.

## Consequences

Foundation v3 independently cleared code review. The enabled parser/replay/turn/
runtime candidate requires a NEW complete independent code review and validation
handoff. The separate pinned-CLI synthetic proof does not establish native/private
model compatibility; see the [operator/evidence boundary](../reasoning-continuation.md). No live provider probe, public release,
credential migration, service automation or delivery is authorized by this ADR.
