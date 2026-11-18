# Reasoning continuation: public provenance and frozen v1 schema

Enabled development candidate (2026-11-01), awaiting final full-feature review.
Foundation-only receipts remain distinct from enabled-feature validation.
Original Go implementations use public protocol facts; no SDK import/runtime or
copied SDK code. Synthetic fixtures are not live/private Codex compatibility proof.
No locally verified cryptographic commit-signature claim is made.

## Official generated schema

Repository `openai/openai-python`, pinned commit
`0dcd904bc2fed26c737d9740bca5f6b0ecc4cb7f`.
Immutable prefix:
`https://raw.githubusercontent.com/openai/openai-python/0dcd904bc2fed26c737d9740bca5f6b0ecc4cb7f/src/openai/types/responses/`.
Root independently fetched exact bytes and matched the Git tree's blobs below.
An earlier research cache had a duplicated generator header; those bodies are NOT
attributed these hashes and are preserved separately, not silently corrected.
Generated types establish public shapes, not event ordering/private extensions.

| File | Git blob | Bytes/lines | Relevant lines |
|---|---|---|---|
| response_reasoning_item.py | 8744204522b660d0ec5bb1506ee2b983ba7b88de |2193/68|11–68|
| response_reasoning_item_param.py | fb1196d30dc9eb4838ff072001e6e11a4c97ae89 |2306/68|11–68|
| response_output_item_added_event.py |36ac0e2245abc4ddb2509bcacad1e4d56dfc9ab3|937/30|11–30|
| response_output_item_done_event.py |7609337c1c7f26f49a237e2698df126f7983e948|706/24|11–24|
| response_reasoning_summary_part_added_event.py |1132070566d3ecd9810e2924bf4c3857551d046b|1112/39|10–39|
| response_reasoning_summary_part_done_event.py |9ba0f56e3b91fba1c3ac65b05afdec8413662630|1343/47|11–47|
| response_reasoning_summary_text_delta_event.py |56f93db07e2b8323943518baecdd06adc9eaaacd|915/29|10–29|
| response_reasoning_summary_text_done_event.py |c7c2ac5812e4b9999bf5059c39f73d6bbd983024|895/29|10–29|
| response_reasoning_text_delta_event.py |c75caacedb4c7ce26d84b36b7dc600c9ea197de4|902/29|10–29|
| response_reasoning_text_done_event.py |4dc39bb122aad9a031056ad0407d59513f34365a|842/29|10–29|
| response_output_item.py |cd738d6512db45c24762619f665a8d9579f41d4b|11314/365|333–365 (reasoning 342)|

### Independently fetched ordinary item schema (same immutable SDK pin)

ROOT fetched and Git-blob-verified these additional six public files. Only public
shape facts inform original Go code; these bodies are not imported/runtime code.

| File | Git blob | Bytes/lines |
|---|---|---|
|response_output_message.py|d041df641539525582f8e4ddcaa861bb88e81695|1565/44|
|response_output_message_param.py|4552d9bf1ff34bb37453e2409658795cac80f54b|1602/44|
|response_output_text.py|e06f14279a2d92d4a28e9b452f52b6ae286e01f3|3179/131|
|response_output_text_param.py|836f28a9a2bf0515bc0034741a2e58d31854928d|3482/129|
|response_function_tool_call.py|c51f68056113fadecd154f44ee220e381421349a|1880/64|
|response_function_tool_call_param.py|096a6391eb92462c3b50058453e405ad5c2cc2a6|1901/67|

Protected output-message replay requires id/type/assistant role/content/status;
final status must be completed. Preserve optional nullable phase (commentary or
final_answer), including omission vs null. Every output_text part requires text
and annotations. The supported finite subset is EMPTY annotations only; nonempty,
unknown, malformed annotations or extra metadata cannot be silently dropped on
protected publication. Optional logprobs support omitted/null/empty array only:
omit output null on nonnullable INPUT replay, preserve empty array. No annotation
or probability values are invented, and no new public Atuin SSE fields are added.

Function caller supports omitted/null/exact `{type:direct}` only; program origin,
unknown nested keys and malformed values are rejected immediately on every
ADDED/DONE/final snapshot, including nonreasoning/raw entry points. Async supports
omitted/null/false only; replay omits output-null async and preserves false. TRUE
or malformed async is never interpreted as an ordinary local tool. Nonnull
namespace remains unsupported. Function status is optional on OUTPUT; known-null
status is omitted on nonnullable INPUT. No hosted/program execution is introduced.

Ordinary ADDED metadata is provisional (in_progress may become completed).
Authoritative DONE presence/digests, role/status/phase/annotations/logprobs and
caller/async must reconcile with final snapshots; omission/change cannot erase
an unsupported execution origin or incomplete signal. Checkpoints retain only
bounded IDs/presence/digests, NEVER each complete encrypted/summary/content DONE
payload; final response.completed supplies all replay bytes in its bounded frame.
Index bounds precede ALL arithmetic/lookups/empty paths for hidden part helpers.
Ordinary checkpoints are minimal per kind, not copies of decoded outputItem:
message inapplicable call strings/arguments/raw slices are discarded after fixed
identity/digest validation, while function-only bounded arguments remain. All
excluded payload metadata digests authenticate required presence and TYPE as well
as separate text hashes. Authoritative DONE validation failures accumulate even
for missing fields; provisional missing ADDED fields may still be completed.
Every sticky/unsupported/execution failure bit is monotonic across later fields,
including omitted role. No raw legacy reconciliation is silently removed.
Ephemeral initial/metadata digests stream a length/type/count-delimited grammar
(decoded strings, exact numbers, sorted object keys, ordered arrays), not whole
large JSON scratch encodings that could remain in stdlib encoder pools. Legacy
initial decoded-field equality and nil-vs-empty content remain explicit controls;
this changes no sealed/public-history/key/credential format.

Unreleased private codec/envelope is now aic-record2/aic-sea2 with explicit
phase/caller/async/logprob presence scalars. Old layout is rejected, not migrated
or decoded as the new format. Projection/policy versions advance; key.bundle
(aic-key1), credential JSON, namespace and public 90-byte handle framing do not.

### Item/part requiredness and replay transform

Required means present/nonnull with the exact type, not implicit coercion.

| Field | OUTPUT reasoning | INPUT replay |
|---|---|---|
|id|required string|required string|
|type|required literal reasoning|same|
|summary|required nonnull array of summary_text parts (empty valid)|required array|
|content|optional null or reasoning_text part array|optional NONNULL array|
|encrypted_content|optional null or string|optional null or string|
|status|optional null or in_progress/completed/incomplete|optional NONNULL same enum|

Each part has ONLY required literal type and string text. Output missing/null/value
presence is retained for completed snapshot reconciliation. Canonical replay omits
known-null content/status, preserving meaningful values/order/IDs and encrypted
presence/null/value. This is not byte-identical raw JSON replay. Locally require
nonempty final encrypted string for every required executable reasoning item;
otherwise reasoning_state_missing before tools (not malformed kind).

### Exact finite events

All rows require nonnull `type` equal to the exact name and `sequence_number:int`.
Other fields below are required/nonnull; only listed optional fields are allowed.

| Event | Other fields | Optional |
|---|---|---|
|response.output_item.added|output_index:int,item:output item|none|
|response.output_item.done|output_index:int,item:output item|none|
|response.reasoning_summary_part.added|item_id:string,output_index:int,summary_index:int,part:summary_text part|none|
|response.reasoning_summary_part.done|item_id:string,output_index:int,summary_index:int,part:summary_text part|status:null or literal incomplete|
|response.reasoning_summary_text.delta|item_id:string,output_index:int,summary_index:int,delta:string|none|
|response.reasoning_summary_text.done|item_id:string,output_index:int,summary_index:int,text:string|none|
|response.reasoning_text.delta|item_id:string,output_index:int,content_index:int,delta:string|none|
|response.reasoning_text.done|item_id:string,output_index:int,content_index:int,text:string|none|

Bridge envelope additionally permits ONLY the existing optional response_id under
existing identity checks. New strict wrapper rules apply to reasoning, not
unrelated legacy event acceptance. Unknown fields/aliases, nonnegative index/work
bounds, strict Unicode/duplicates, final reconciliation, finite status acceptance
and ordering are LOCAL hardening, not SDK guarantees. No reasoning-prefix wildcard.

Added opaque content (even nonempty) is provisional: A→entirely different done B
is allowed; no append/prefix/equality requirement. Done and explicit completed
output are authoritative and must reconcile meaningful bytes AND output presence.
Final-only completed snapshots are supported. Authoritative missing/null status
is locally complete; completed accepted; in_progress/incomplete rejected. An
observed incomplete item or summary-part.done incomplete interrupts/holds all
tools. Summary-part.done missing/null status is normal. Keep identity/global
sequence retransmission, sparse bounds/layout and text digest checks. Never emit
summary/reasoning text to ordinary sink/logs; no per-delta archive. Existing shared
128 items/aggregate parts,1MiB frames,full output/usage/batch gate remain unchanged.
Unknown hosted/execution/approval/namespaces and nonempty lifecycle output stay
closed. Raw Stream still rejects ALL reasoning; only the prepared generation
transaction accepts these finite supported shapes and may publish protected IDs.

## Pinned public Rust client

`atuinsh/atuin@4ca8b77c8a526b8c294eb864a2b74c95c685c983`, immutable prefix
`https://raw.githubusercontent.com/atuinsh/atuin/4ca8b77c8a526b8c294eb864a2b74c95c685c983/crates/atuin-ai/src/`.

| File | Git blob | Facts |
|---|---|---|
|context_window.rs|93761533a96e4b230d3714d55e543b188a0ad2cd|35–150:200000 serialized UTF8 BYTES (String.len), first ONE frozen turn+whole-turn tail+exact marker, latest retained|
|tui/state.rs|63075230cebc48286b9705d73a620661c8a365d8|8–49,81–224:serde_json Value args, text+consecutive calls grouping, singleton user results, user/system/skill text indistinguishable|
|stream.rs|10947bb50dcf008beb9ba9a132682fb9d182bcb5|119–147,185–234:history/session/header identity, String IDs, Value args|
|fsm/mod.rs|d74c6c2942bb92250f9f1a54e33e748a7f6dc3ca|473–520,624–651,792–809,931–938,1084–1092,1152–1187:trim_start only, asynchronous results can precede later calls, continuation waits done+all tools|

Rust whitespace set: U+0009..000D,0020,0085,00A0,1680,2000..200A,2028,2029,202F,
205F,3000. U+200B/FEFF are NOT whitespace. A separate faithful pinned-client
synthetic fixture now checks exact protected ID/integer replay and two actual
server-process restarts. That execution proves nothing about unknown installed
builds or live/private provider behavior. Window marker is exactly:
`[Earlier conversation context was omitted to fit within the context window. The conversation continues below.]`.
Strict binding intentionally rejects changed protected prefixes after windowing,
policy/model/session edits even below TTL/caps; see ADR 0005 for /new vs attended
global-key recovery and stable rollback with SAME login/NEW conversation.

## Behavioral public references and evidence boundary

- https://developers.openai.com/api/docs/guides/function-calling : reasoning
  returned with reasoning-model tool calls accompanies corresponding outputs.
- https://platform.openai.com/docs/guides/reasoning : recommended broader state
  retention is distinct from mandatory tool-output continuation.

No none/effort setting, private Codex extension, native/private-model success,
October 30 root cause, installed-client execution, universal all-turn support or
secure-erasure claim follows from this research. Synthetic fixtures are original
facts-only test data. Enabled provider/parser/runtime/CLI validation belongs to
`dist/reasoning-full-integration-validation-v3/`, not the CODE-HOLD534422/fb9 candidates
or earlier foundation receipts;
see the [finite feature/recovery guide](../reasoning-continuation.md).
