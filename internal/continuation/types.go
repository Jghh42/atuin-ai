// Package continuation implements bounded private continuation primitives.
// Production uses these only through the authenticated prepared transaction.
package continuation

import (
	"fmt"
	"io"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

type private struct{}

func (private) String() string   { return "[private continuation]" }
func (private) GoString() string { return "[private continuation]" }

func (private) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "[private continuation]") }
func (private) MarshalJSON() ([]byte, error) { return []byte(`"[private continuation]"`), nil }

type publicHandlePayload struct {
	value string
}

func (h PublicHandle) Value() string {
	if h.publicHandlePayload == nil {
		return ""
	}
	return h.value
}
func NewBackendCallID(value string) BackendCallID {
	return newBackendCallID(backendCallIDPayload{value: value})
}
func (b BackendCallID) Value() string {
	if b.backendCallIDPayload == nil {
		return ""
	}
	return b.value
}
func NewScopeInput(id credentials.Identity, model, session string, policy Commitment) ScopeInput {
	return newScopeInput(scopeInputPayload{identity: id, model: model, session: session, policy: policy})
}

type backendCallIDPayload struct {
	value string
}
type commitmentPayload struct {
	bytes [32]byte
}
type snapshotIDPayload struct {
	bytes [16]byte
}

type scopeInputPayload struct {
	identity       credentials.Identity
	model, session string
	policy         Commitment
}
type prefixPayload struct {
	messages uint32
	digest   Commitment
}
type textExpectationPayload struct {
	bytes  uint32
	digest Commitment
}
type spanPayload struct {
	bytes  uint32
	digest [32]byte
}

// TextRecipe restores raw provider text from discarded leading bytes plus the
// verified client's retained bytes, slicing spans in ORIGINAL item/part order.
type textRecipePayload struct {
	leading   []byte
	spans     []Span
	rawBytes  uint32
	rawDigest [32]byte
}
type callPayload struct {
	handle          PublicHandle
	backend         BackendCallID
	name            string
	publicArguments Commitment
}
type partPayload struct {
	kind             uint8
	text             string
	span             uint32
	logprobsPresence uint8
}

// Item kinds: 1 ordinary message, 2 function_call, 3 reasoning. Ordinary
// content uses span references, never a saved transcript. Reasoning summaries
// and opaque state are private. Presence: 0 missing, 1 null, 2 value.
type itemPayload struct {
	kind                                               uint8
	iD, callID, name, arguments, encrypted, status     string
	contentPresence, encryptedPresence, statusPresence uint8
	summary, content                                   []Part
	phase                                              string
	phasePresence, callerPresence, asyncPresence       uint8
}
type segmentPayload struct {
	snapshot         SnapshotID
	created, expires int64
	prefix           Prefix
	text             TextExpectation
	recipe           TextRecipe
	calls            []Call
	items            []Item
}
type recordPayload struct {
	snapshot         SnapshotID
	scope            Commitment
	created, expires int64
	segments         []Segment
}

// Match contains only request-owned reconstruction data, never persisted user
// results/text. Results are message indices in their actual incoming order.
type matchPayload struct {
	start, end int
	text       string
	results    []int
}

type Code uint8

const (
	HandleInvalid Code = iota + 1
	Missing
	Expired
	Corrupt
	BindingMismatch
	Unavailable
	Limit
	ClockInvalid
	CommitFailed
	LifecycleChanged
	ProjectionUnsupported
	ReasoningStateMissing
	ReasoningNoVisibleOutput
)

type errorPayload struct {
	code Code
}

func (e Error) Error() string { return "continuation unavailable" }
func (e Error) Code() Code {
	if e.errorPayload == nil || e.code > ReasoningNoVisibleOutput {
		return 0
	}
	return e.code
}
func failure(c Code) error { return newError(errorPayload{code: c}) }

const (
	MaxRecordBytes        = 2 << 20
	MaxLeadingBytes       = 1 << 20
	MaxSegments           = 128
	MaxItems              = 1024
	MaxParts              = 4096
	MaxSpans              = 4096
	MaxCalls              = 32
	MaxRecords            = 128
	MaxStoreBytes   int64 = 64 << 20
	MaxEntries            = 132
	MaxScopes             = 4
	MaxTokens             = 262144
	TTLMillis       int64 = 24 * 60 * 60 * 1000
	minMillis       int64 = 1577836800000   // 2020-01-01
	maxMillis       int64 = 253402300799999 // 9999-12-31
)
const ProjectionVersion = "continuation-v2/rust-leading25/grammar1/int64-u64/message-metadata2"
const PolicyVersion = "prompt1/registry1/projection2"
