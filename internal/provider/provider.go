// Package provider defines the one-generation boundary. Implementations must
// succeed only on explicit provider completion, never on EOF. Tool calls are
// delivered as one completed batch, never as executable argument deltas.
package provider

import (
	"context"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

// Request carries normalized full history; SessionID is never provider state.
type Request struct{ Conversation atuin.Request }

type EventKind string

const (
	TextDelta EventKind = "text_delta"
	ToolCalls EventKind = "tool_calls"
)

type Event struct {
	Kind  EventKind
	Text  string
	Calls []tools.Call
}

type Usage struct{ InputTokens, OutputTokens int64 }

// Stream makes one generation, without automatic retries. Sink failure must
// cancel upstream work immediately. Codex does not implement this yet: Phase 4
// provides only request projection/HTTP opening; Phase 5 owns stream parsing.
type Provider interface {
	Stream(context.Context, Request, func(Event) error) (Usage, error)
}
