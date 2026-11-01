package continuation

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

// Opaque ownership boundary: no exported payload fields. In Go 1.27 invalid
// formatting verbs suppress formatting methods; the outer object therefore
// contains only an UNEXPORTED payload pointer, never traversed at nested depth.
// Backing payloads are package-private trusted codec access, not presentations.
type PublicHandle struct {
	private
	*publicHandlePayload
}

func newPublicHandle(v publicHandlePayload) PublicHandle {
	return PublicHandle{publicHandlePayload: &v}
}
func clonePublicHandle(v PublicHandle) PublicHandle {
	if v.publicHandlePayload == nil {
		return newPublicHandle(publicHandlePayload{})
	}
	return newPublicHandle(*v.publicHandlePayload)
}

type BackendCallID struct {
	private
	*backendCallIDPayload
}

func newBackendCallID(v backendCallIDPayload) BackendCallID {
	return BackendCallID{backendCallIDPayload: &v}
}
func cloneBackendCallID(v BackendCallID) BackendCallID {
	if v.backendCallIDPayload == nil {
		return newBackendCallID(backendCallIDPayload{})
	}
	return newBackendCallID(*v.backendCallIDPayload)
}

type Commitment struct {
	private
	*commitmentPayload
}

func newCommitment(v commitmentPayload) Commitment {
	return Commitment{commitmentPayload: &v}
}
func cloneCommitment(v Commitment) Commitment {
	if v.commitmentPayload == nil {
		return newCommitment(commitmentPayload{})
	}
	return newCommitment(*v.commitmentPayload)
}

type SnapshotID struct {
	private
	*snapshotIDPayload
}

func newSnapshotID(v snapshotIDPayload) SnapshotID {
	return SnapshotID{snapshotIDPayload: &v}
}
func cloneSnapshotID(v SnapshotID) SnapshotID {
	if v.snapshotIDPayload == nil {
		return newSnapshotID(snapshotIDPayload{})
	}
	return newSnapshotID(*v.snapshotIDPayload)
}

type ScopeInput struct {
	private
	*scopeInputPayload
}

func newScopeInput(v scopeInputPayload) ScopeInput {
	v.policy = cloneCommitment(v.policy)
	return ScopeInput{scopeInputPayload: &v}
}

type Prefix struct {
	private
	*prefixPayload
}

func newPrefix(v prefixPayload) Prefix {
	v.digest = cloneCommitment(v.digest)
	return Prefix{prefixPayload: &v}
}
func clonePrefix(v Prefix) Prefix {
	if v.prefixPayload == nil {
		return newPrefix(prefixPayload{})
	}
	return newPrefix(*v.prefixPayload)
}

type TextExpectation struct {
	private
	*textExpectationPayload
}

func newTextExpectation(v textExpectationPayload) TextExpectation {
	v.digest = cloneCommitment(v.digest)
	return TextExpectation{textExpectationPayload: &v}
}
func cloneTextExpectation(v TextExpectation) TextExpectation {
	if v.textExpectationPayload == nil {
		return newTextExpectation(textExpectationPayload{})
	}
	return newTextExpectation(*v.textExpectationPayload)
}

type Span struct {
	private
	*spanPayload
}

func newSpan(v spanPayload) Span {
	return Span{spanPayload: &v}
}
func cloneSpan(v Span) Span {
	if v.spanPayload == nil {
		return newSpan(spanPayload{})
	}
	return newSpan(*v.spanPayload)
}
func makeSpan(n int, capacity ...int) []Span {
	c := n
	if len(capacity) > 0 {
		c = capacity[0]
	}
	v := make([]Span, n, c)
	for j := range v {
		v[j] = newSpan(spanPayload{})
	}
	return v
}

type TextRecipe struct {
	private
	*textRecipePayload
}

func newTextRecipe(v textRecipePayload) TextRecipe {
	v.leading = append([]byte(nil), v.leading...)
	v.spans = append([]Span(nil), v.spans...)
	for j := range v.spans {
		v.spans[j] = cloneSpan(v.spans[j])
	}
	return TextRecipe{textRecipePayload: &v}
}
func cloneTextRecipe(v TextRecipe) TextRecipe {
	if v.textRecipePayload == nil {
		return newTextRecipe(textRecipePayload{})
	}
	return newTextRecipe(*v.textRecipePayload)
}

type Call struct {
	private
	*callPayload
}

func newCall(v callPayload) Call {
	v.handle = clonePublicHandle(v.handle)
	v.backend = cloneBackendCallID(v.backend)
	v.publicArguments = cloneCommitment(v.publicArguments)
	return Call{callPayload: &v}
}
func cloneCall(v Call) Call {
	if v.callPayload == nil {
		return newCall(callPayload{})
	}
	return newCall(*v.callPayload)
}
func makeCall(n int, capacity ...int) []Call {
	c := n
	if len(capacity) > 0 {
		c = capacity[0]
	}
	v := make([]Call, n, c)
	for j := range v {
		v[j] = newCall(callPayload{})
	}
	return v
}

type Part struct {
	private
	*partPayload
}

func newPart(v partPayload) Part {
	return Part{partPayload: &v}
}
func clonePart(v Part) Part {
	if v.partPayload == nil {
		return newPart(partPayload{})
	}
	return newPart(*v.partPayload)
}
func makePart(n int, capacity ...int) []Part {
	c := n
	if len(capacity) > 0 {
		c = capacity[0]
	}
	v := make([]Part, n, c)
	for j := range v {
		v[j] = newPart(partPayload{})
	}
	return v
}

type Item struct {
	private
	*itemPayload
}

func newItem(v itemPayload) Item {
	v.summary = append([]Part(nil), v.summary...)
	for j := range v.summary {
		v.summary[j] = clonePart(v.summary[j])
	}
	v.content = append([]Part(nil), v.content...)
	for j := range v.content {
		v.content[j] = clonePart(v.content[j])
	}
	return Item{itemPayload: &v}
}
func cloneItem(v Item) Item {
	if v.itemPayload == nil {
		return newItem(itemPayload{})
	}
	return newItem(*v.itemPayload)
}
func makeItem(n int, capacity ...int) []Item {
	c := n
	if len(capacity) > 0 {
		c = capacity[0]
	}
	v := make([]Item, n, c)
	for j := range v {
		v[j] = newItem(itemPayload{})
	}
	return v
}

type Segment struct {
	private
	*segmentPayload
}

func newSegment(v segmentPayload) Segment {
	v.snapshot = cloneSnapshotID(v.snapshot)
	v.prefix = clonePrefix(v.prefix)
	v.text = cloneTextExpectation(v.text)
	v.recipe = cloneTextRecipe(v.recipe)
	v.calls = append([]Call(nil), v.calls...)
	for j := range v.calls {
		v.calls[j] = cloneCall(v.calls[j])
	}
	v.items = append([]Item(nil), v.items...)
	for j := range v.items {
		v.items[j] = cloneItem(v.items[j])
	}
	return Segment{segmentPayload: &v}
}
func cloneSegment(v Segment) Segment {
	if v.segmentPayload == nil {
		return newSegment(segmentPayload{})
	}
	return newSegment(*v.segmentPayload)
}
func makeSegment(n int, capacity ...int) []Segment {
	c := n
	if len(capacity) > 0 {
		c = capacity[0]
	}
	v := make([]Segment, n, c)
	for j := range v {
		v[j] = newSegment(segmentPayload{})
	}
	return v
}

type Record struct {
	private
	*recordPayload
}

func newRecord(v recordPayload) Record {
	v.snapshot = cloneSnapshotID(v.snapshot)
	v.scope = cloneCommitment(v.scope)
	v.segments = append([]Segment(nil), v.segments...)
	for j := range v.segments {
		v.segments[j] = cloneSegment(v.segments[j])
	}
	return Record{recordPayload: &v}
}
func cloneRecord(v Record) Record {
	if v.recordPayload == nil {
		return newRecord(recordPayload{})
	}
	return newRecord(*v.recordPayload)
}

type Match struct {
	private
	*matchPayload
}

func newMatch(v matchPayload) Match {
	v.results = append([]int(nil), v.results...)
	return Match{matchPayload: &v}
}
func makeMatch(n int, capacity ...int) []Match {
	c := n
	if len(capacity) > 0 {
		c = capacity[0]
	}
	v := make([]Match, n, c)
	for j := range v {
		v[j] = newMatch(matchPayload{})
	}
	return v
}

type Error struct {
	private
	*errorPayload
}

func newError(v errorPayload) Error {
	return Error{errorPayload: &v}
}

type keys struct {
	private
	*keysPayload
}

func newKeys(v keysPayload) keys {
	return keys{keysPayload: &v}
}

type handleParts struct {
	private
	*handlePartsPayload
}

func newHandleParts(v handlePartsPayload) handleParts {
	v.snapshot = cloneSnapshotID(v.snapshot)
	return handleParts{handlePartsPayload: &v}
}

type Store struct {
	private
	*storePayload
}

func newStore(v storePayload) Store {
	if v.mu == nil {
		v.mu = new(sync.Mutex)
	}
	if v.floor == nil {
		v.floor = new(atomic.Int64)
	}
	return Store{storePayload: &v}
}

type Lease struct {
	private
	*leasePayload
}

func newLease(v leasePayload) Lease {
	return Lease{leasePayload: &v}
}

type leaseContext struct {
	private
	*leaseContextPayload
}

func newLeaseContext(v leaseContextPayload) leaseContext {
	return leaseContext{leaseContextPayload: &v}
}

type emitContext struct {
	private
	*emitContextPayload
}

func newEmitContext(v emitContextPayload) emitContext {
	return emitContext{emitContextPayload: &v}
}

type TextCapture struct {
	private
	*textCapturePayload
}

func newTextCapture(v textCapturePayload) TextCapture {
	v.leading = append([]byte(nil), v.leading...)
	return TextCapture{textCapturePayload: &v}
}

type encoder struct {
	private
	*encoderPayload
}

func newEncoder(v encoderPayload) encoder {
	v.b = append([]byte(nil), v.b...)
	return encoder{encoderPayload: &v}
}

type decoder struct {
	private
	*decoderPayload
}

func newDecoder(v decoderPayload) decoder {
	v.b = append([]byte(nil), v.b...)
	return decoder{decoderPayload: &v}
}
func ownedPointer[T any](v T) *T { return &v }
func sameSnapshot(a, b SnapshotID) bool {
	if a.snapshotIDPayload == nil {
		a = newSnapshotID(snapshotIDPayload{})
	}
	if b.snapshotIDPayload == nil {
		b = newSnapshotID(snapshotIDPayload{})
	}
	return a.bytes == b.bytes
}

// Pointer LogValue methods are explicitly nil-safe. Value presentations use
// MarshalJSON/Formatter; no implicit nil value-receiver wrapper can panic in slog.
func (*PublicHandle) LogValue() slog.Value    { return slog.StringValue("[private continuation]") }
func (*BackendCallID) LogValue() slog.Value   { return slog.StringValue("[private continuation]") }
func (*Commitment) LogValue() slog.Value      { return slog.StringValue("[private continuation]") }
func (*SnapshotID) LogValue() slog.Value      { return slog.StringValue("[private continuation]") }
func (*ScopeInput) LogValue() slog.Value      { return slog.StringValue("[private continuation]") }
func (*Prefix) LogValue() slog.Value          { return slog.StringValue("[private continuation]") }
func (*TextExpectation) LogValue() slog.Value { return slog.StringValue("[private continuation]") }
func (*Span) LogValue() slog.Value            { return slog.StringValue("[private continuation]") }
func (*TextRecipe) LogValue() slog.Value      { return slog.StringValue("[private continuation]") }
func (*Call) LogValue() slog.Value            { return slog.StringValue("[private continuation]") }
func (*Part) LogValue() slog.Value            { return slog.StringValue("[private continuation]") }
func (*Item) LogValue() slog.Value            { return slog.StringValue("[private continuation]") }
func (*Segment) LogValue() slog.Value         { return slog.StringValue("[private continuation]") }
func (*Record) LogValue() slog.Value          { return slog.StringValue("[private continuation]") }
func (*Match) LogValue() slog.Value           { return slog.StringValue("[private continuation]") }
func (*Error) LogValue() slog.Value           { return slog.StringValue("[private continuation]") }
func (*keys) LogValue() slog.Value            { return slog.StringValue("[private continuation]") }
func (*handleParts) LogValue() slog.Value     { return slog.StringValue("[private continuation]") }
func (*Store) LogValue() slog.Value           { return slog.StringValue("[private continuation]") }
func (*Lease) LogValue() slog.Value           { return slog.StringValue("[private continuation]") }
func (*leaseContext) LogValue() slog.Value    { return slog.StringValue("[private continuation]") }
func (*emitContext) LogValue() slog.Value     { return slog.StringValue("[private continuation]") }
func (*TextCapture) LogValue() slog.Value     { return slog.StringValue("[private continuation]") }
func (*encoder) LogValue() slog.Value         { return slog.StringValue("[private continuation]") }
func (*decoder) LogValue() slog.Value         { return slog.StringValue("[private continuation]") }
