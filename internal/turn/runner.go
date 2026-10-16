// Package turn owns the lifecycle of exactly one upstream generation.
package turn

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

type Event struct {
	Name string
	Data any
}
type EventSink interface {
	Emit(context.Context, Event) error
}
type SinkFunc func(context.Context, Event) error

func (f SinkFunc) Emit(ctx context.Context, e Event) error { return f(ctx, e) }

var errEvents = errors.New("invalid provider events")

// Run never executes tools, retries, or continues a generation. Sink errors are
// sticky: cancel immediately and never attempt another downstream write.
func Run(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink) {
	run(parent, p, req, sink, config.GenerationTimeout)
}

func run(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	writable := true
	emit := func(name string, data any) bool {
		if !writable || ctx.Err() != nil {
			return false
		}
		if err := sink.Emit(ctx, Event{name, data}); err != nil {
			writable = false
			cancel()
			return false
		}
		return true
	}
	status := func(state string) bool {
		return emit("status", struct {
			State string `json:"state"`
		}{state})
	}
	if !status("processing") || !status("thinking") {
		return
	}
	var mu sync.Mutex
	closed, batchSeen := false, false
	var eventErr error
	var calls []tools.Call
	usage, streamErr := p.Stream(ctx, provider.Request{Conversation: req}, func(e provider.Event) error {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return errEvents
		}
		if eventErr != nil {
			return eventErr
		}
		switch e.Kind {
		case provider.TextDelta:
			if batchSeen || len(e.Calls) != 0 || len(e.Text) > config.MaxProviderEventBytes || !utf8.ValidString(e.Text) {
				eventErr = errEvents
				break
			}
			if !emit("text", struct {
				Content string `json:"content"`
			}{e.Text}) {
				eventErr = errEvents
			}
		case provider.ToolCalls:
			if batchSeen || e.Text != "" || len(e.Calls) > config.MaxCallsPerResponse {
				eventErr = errEvents
				break
			}
			batchSeen = true
			// Own the bytes even if a faulty provider reuses its callback buffers.
			for _, c := range e.Calls {
				if len(c.Input) > config.MaxCallArgumentsBytes {
					eventErr = errEvents
					break
				}
				c.Input = append(json.RawMessage(nil), c.Input...)
				calls = append(calls, c)
			}
		default:
			eventErr = errEvents
		}
		if eventErr != nil {
			cancel()
		}
		return eventErr
	})
	mu.Lock()
	defer mu.Unlock()
	closed = true
	if !writable || parent.Err() != nil {
		return
	}
	// Event validation cancels upstream but a writable downstream still gets its
	// one generic terminal. Use the parent context for that final attempt.
	generationErr := ctx.Err()
	ctx = parent
	fail := func() {
		emit("error", struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}{"LLM request failed, please retry", "upstream_error"})
	}
	if streamErr != nil || eventErr != nil || generationErr != nil {
		fail()
		return
	}
	batch, err := tools.ValidateBatch(calls, req.Config.Capabilities)
	if err != nil {
		fail()
		return
	}
	seen := make(map[string]bool)
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_use" {
				seen[b.ID] = true
			}
		}
	}
	for _, c := range batch.Calls {
		if seen[c.ID] {
			fail()
			return
		}
	}
	for _, c := range batch.Calls {
		if !emit("tool_call", c) {
			return
		}
	}
	if batch.Kind == tools.LocalTools && !status("waiting_for_tools") {
		return
	}
	emit("done", struct {
		SessionID string `json:"session_id"`
		Usage     any    `json:"usage"`
	}{req.SessionID, usagePayload(usage)})
}

// Unknown counts remain null, including cache creation (not supplied by Codex).
// Costs are always null: subscription tokens are not a price/credit estimate.
func usagePayload(u provider.Usage) map[string]any {
	count := func(v int64, known bool) any {
		if known {
			return v
		}
		return nil
	}
	return map[string]any{"input_tokens": count(u.InputTokens, u.InputKnown), "output_tokens": count(u.OutputTokens, u.OutputKnown), "total_tokens": count(u.TotalTokens, u.TotalKnown), "cached_tokens": count(u.CachedTokens, u.CachedKnown), "cache_creation_tokens": nil, "input_cost": nil, "output_cost": nil, "total_cost": nil, "provider_cost": nil}
}
