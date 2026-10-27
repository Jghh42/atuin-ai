package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// Phase and Category are closed diagnostic vocabularies, not provider text.
type Phase uint8

const (
	PhaseUnknown Phase = iota
	PhaseRequest
	PhaseCredentials
	PhaseTransport
	PhaseHTTP
	PhaseStream
	PhaseTurn
)

func (p Phase) String() string {
	switch p {
	case PhaseRequest:
		return "request"
	case PhaseCredentials:
		return "credentials"
	case PhaseTransport:
		return "transport"
	case PhaseHTTP:
		return "http"
	case PhaseStream:
		return "stream"
	case PhaseTurn:
		return "turn"
	default:
		return "unknown"
	}
}

func (p Phase) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, p.String()) }
func (p Phase) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }
func (p Phase) LogValue() slog.Value         { return slog.StringValue(p.String()) }

type Category uint8

const (
	CategoryUnknown Category = iota
	RequestInvalid
	CredentialsUnavailable
	TransportFailure
	TransportTimeout
	HTTPUnauthorized
	HTTPForbidden
	HTTPRateLimited
	HTTPServerError
	HTTPRejected
	StreamIncomplete
	StreamMalformed
	StreamReadFailure
	StreamReadTimeout
	UpstreamError
	UpstreamFailed
	UpstreamIncomplete
	Refusal
	UnsupportedOutput
	InactivityTimeout
	GenerationTimeout
	EventsInvalid
	ToolBatchInvalid
	HistoryCallIDCollision
	ProviderFailure
)

func (c Category) String() string {
	names := [...]string{"unknown", "request_invalid", "credentials_unavailable", "transport_failure", "transport_timeout", "http_unauthorized", "http_forbidden", "http_rate_limited", "http_server_error", "http_rejected", "stream_incomplete", "stream_malformed", "stream_read_failure", "stream_read_timeout", "upstream_error", "upstream_failed", "upstream_incomplete", "refusal", "unsupported_output", "inactivity_timeout", "generation_timeout", "events_invalid", "tool_batch_invalid", "history_call_id_collision", "provider_failure"}
	if int(c) >= len(names) {
		return "unknown"
	}
	return names[c]
}

func (c Category) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, c.String()) }
func (c Category) MarshalJSON() ([]byte, error) { return json.Marshal(c.String()) }
func (c Category) LogValue() slog.Value         { return slog.StringValue(c.String()) }

// Failure is an immutable, content-free snapshot. Zero status means unavailable.
type Failure struct {
	phase    Phase
	category Category
	status   uint16
}

func NewFailure(p Phase, c Category, status int) Failure {
	if p > PhaseTurn {
		p = PhaseUnknown
	}
	if c > ProviderFailure {
		c = CategoryUnknown
	}
	if status < 100 || status > 599 {
		status = 0
	}
	return Failure{p, c, uint16(status)}
}
func (f Failure) Phase() Phase       { return f.phase }
func (f Failure) Category() Category { return f.category }
func (f Failure) HTTPStatus() int    { return int(f.status) }
func (f Failure) statusValue() any {
	if f.status == 0 {
		return nil
	}
	return int(f.status)
}
func (f Failure) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Phase    string `json:"phase"`
		Category string `json:"category"`
		Status   any    `json:"upstream_http_status"`
	}{f.phase.String(), f.category.String(), f.statusValue()})
}
func (f Failure) LogValue() slog.Value {
	return slog.GroupValue(slog.String("phase", f.phase.String()), slog.String("category", f.category.String()), slog.Any("upstream_http_status", f.statusValue()))
}
func (f Failure) Format(s fmt.State, _ rune) { b, _ := f.MarshalJSON(); _, _ = s.Write(b) }

type failureError struct {
	failure Failure
	cause   error
}

// WithFailure retains only a trusted, fixed sentinel/context cause. Callers must
// discard transport, credential and provider errors before using this wrapper.
// Its presentation never invokes the cause's Error, String, Format or LogValue.
func WithFailure(cause error, f Failure) error { return &failureError{f, cause} }
func (e *failureError) Error() string          { return "generation failed" }
func (e *failureError) Unwrap() error          { return e.cause }
func (e *failureError) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "generation failed")
	e.failure.Format(s, 'v')
}
func (e *failureError) MarshalJSON() ([]byte, error) { return e.failure.MarshalJSON() }
func (e *failureError) LogValue() slog.Value         { return e.failure.LogValue() }

// WalkErrors bounds TOTAL inspected nodes/edges (including joined breadth),
// storage and cycles. It never calls Error/Is/As. Unwrap implementations are
// trusted to return; like any synchronous interface call they cannot be timed
// out here. Panicking implementations stop inspection without exposing values.
func WalkErrors(err error, visit func(error) bool) {
	defer func() { _ = recover() }()
	const budget = 64
	type frame struct {
		children []error
		next     int
	}
	var stack [budget]frame
	depth, work := 0, 0
	for err != nil && work < budget {
		work++
		if visit(err) {
			return
		}
		switch e := err.(type) {
		case interface{ Unwrap() []error }:
			children := e.Unwrap()
			if len(children) != 0 {
				stack[depth] = frame{children, 1}
				depth++
				err = children[0]
				if err != nil {
					continue
				}
				work++ // A nil first child still consumes the total sibling budget.
			}
		case interface{ Unwrap() error }:
			child := e.Unwrap()
			if child != nil {
				err = child
				continue
			}
		}
		err = nil
		for depth > 0 && work < budget {
			f := &stack[depth-1]
			if f.next >= len(f.children) {
				*f = frame{}
				depth--
				continue
			}
			err = f.children[f.next]
			f.next++
			if err != nil {
				break
			}
			work++
		}
	}
}

// FailureOf returns the first typed snapshot in bounded depth-first order.
func FailureOf(err error) (Failure, bool) {
	var f Failure
	found := false
	WalkErrors(err, func(e error) bool {
		if typed, ok := e.(*failureError); ok {
			f, found = typed.failure, true
		}
		return found
	})
	return f, found
}
