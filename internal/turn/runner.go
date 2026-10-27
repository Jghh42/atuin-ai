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

// RunWithObserver preserves Run's wire contract with an optional safe observer.
func RunWithObserver(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, observer FailureObserver) {
	runObserved(parent, p, req, sink, config.GenerationTimeout, observer)
}

func run(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, timeout time.Duration) {
	runObserved(parent, p, req, sink, timeout, nil)
}

func runObserved(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, timeout time.Duration, observer FailureObserver) {
	var report FailureReport
	selected := false
	// Registered first: executes after callback unlock and context cancellation.
	defer func() {
		if selected && observer != nil {
			observe(observer, report)
		}
	}()
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
	invalidEvents := false
	var calls []tools.Call
	started := time.Now()
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
				invalidEvents = true
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
				invalidEvents = true
				eventErr = errEvents
				break
			}
			batchSeen = true
			// Own the bytes even if a faulty provider reuses its callback buffers.
			for _, c := range e.Calls {
				if len(c.Input) > config.MaxCallArgumentsBytes {
					invalidEvents = true
					eventErr = errEvents
					break
				}
				c.Input = append(json.RawMessage(nil), c.Input...)
				calls = append(calls, c)
			}
		default:
			invalidEvents = true
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
	fail := func(f provider.Failure) {
		selected = true
		report = NewFailureReport(f, time.Since(started))
		emit("error", struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}{"LLM request failed, please retry", "upstream_error"})
	}
	if streamErr != nil || eventErr != nil || generationErr != nil {
		f := provider.NewFailure(provider.PhaseTurn, provider.ProviderFailure, 0)
		if invalidEvents {
			f = provider.NewFailure(provider.PhaseTurn, provider.EventsInvalid, 0)
		} else if typed, ok := provider.FailureOf(streamErr); ok {
			f = typed
		} else if generationErr == context.DeadlineExceeded {
			f = provider.NewFailure(provider.PhaseTurn, provider.GenerationTimeout, 0)
		}
		fail(f)
		return
	}
	batch, err := tools.ValidateBatch(calls, req.Config.Capabilities)
	if err != nil {
		fail(provider.NewFailure(provider.PhaseTurn, provider.ToolBatchInvalid, 0))
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
			fail(provider.NewFailure(provider.PhaseTurn, provider.HistoryCallIDCollision, 0))
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
