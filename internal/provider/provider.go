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

// Usage counts tokens, not price/credits. Input includes cached tokens. Known
// flags distinguish an explicit zero from absent upstream accounting. Total is
// derived only when both input and output are known; no other missing counts
// are invented.
type Usage struct {
	InputTokens, OutputTokens, TotalTokens, CachedTokens int64
	InputKnown, OutputKnown, TotalKnown, CachedKnown     bool
}

// Stream makes one generation, without automatic retries. Sink failure must
// cancel upstream work immediately. Implementations never execute tools.
type Provider interface {
	Stream(context.Context, Request, func(Event) error) (Usage, error)
}
