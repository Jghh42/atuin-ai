// Package turn owns the lifecycle of exactly one upstream generation.
package turn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
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

// Run owns one generation, never tools/retries. Writer errors are sticky.
func Run(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink) {
	run(parent, p, req, sink, config.GenerationTimeout)
}
func RunWithObserver(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, observer FailureObserver) {
	runObserved(parent, p, req, sink, config.GenerationTimeout, observer)
}
func RunWithObservers(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, failure FailureObserver, reasoning ReasoningObserver) {
	runObservers(parent, p, req, sink, config.GenerationTimeout, failure, reasoning)
}
func run(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, timeout time.Duration) {
	runObserved(parent, p, req, sink, timeout, nil)
}
func runObserved(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, timeout time.Duration, observer FailureObserver) {
	runObservers(parent, p, req, sink, timeout, observer, nil)
}

func runObservers(parent context.Context, p provider.Provider, req atuin.Request, sink EventSink, timeout time.Duration, observer FailureObserver, reasoningObserver ReasoningObserver) {
	var report FailureReport
	var reasoningReport ReasoningReport
	selected, reasoningSelected := false, false
	// Registered first: terminal -> unlock -> transaction/native cleanup -> cancel
	// -> observers. Synchronous trusted logger latency is intentionally not hidden.
	defer func() {
		if selected && observer != nil {
			observe(observer, report)
		} else if reasoningSelected && reasoningObserver != nil {
			observeReasoning(reasoningObserver, reasoningReport)
		}
	}()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var prepared provider.PreparedGeneration
	defer func() {
		if prepared != nil {
			prepared.Close()
		}
	}()
	writable, terminalAttempted, terminalSucceeded := true, false, false
	var writerError error
	emit := func(bound context.Context, name string, data any) error {
		if !writable {
			if writerError != nil {
				return writerError
			}
			return errEvents
		}
		if e := bound.Err(); e != nil {
			return e
		}
		if terminalAttempted {
			return errEvents
		}
		if name == "done" || name == "error" {
			terminalAttempted = true
		}
		if e := sink.Emit(bound, Event{name, data}); e != nil {
			writerError = e
			writable = false
			cancel()
			return e
		}
		if name == "done" {
			terminalSucceeded = true
		}
		return nil
	}
	status := func(bound context.Context, state string) error {
		return emit(bound, "status", struct {
			State string `json:"state"`
		}{state})
	}
	if status(ctx, "processing") != nil || status(ctx, "thinking") != nil {
		return
	}
	started := time.Now()
	var prepareErr error
	if pp, ok := p.(provider.PreparedProvider); ok {
		prepared, prepareErr = pp.Prepare(ctx, provider.Request{Conversation: req})
		if prepareErr == nil && prepared == nil {
			prepareErr = provider.WithFailure(errEvents, provider.NewFailure(provider.PhaseTurn, provider.EventsInvalid, 0))
		}
	}
	var mu sync.Mutex
	closed, batchSeen, invalidEvents := false, false, false
	var eventErr error
	var calls []tools.Call
	callback := func(e provider.Event) error {
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
			if e := emit(ctx, "text", struct {
				Content string `json:"content"`
			}{e.Text}); e != nil {
				eventErr = e
			}
		case provider.ToolCalls:
			if batchSeen || e.Text != "" || len(e.Calls) > config.MaxCallsPerResponse {
				invalidEvents = true
				eventErr = errEvents
				break
			}
			batchSeen = true
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
	}
	var usage provider.Usage
	streamErr := prepareErr
	if prepareErr == nil {
		if prepared != nil {
			usage, streamErr = prepared.Stream(ctx, callback)
		} else {
			usage, streamErr = p.Stream(ctx, provider.Request{Conversation: req}, callback)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	closed = true
	if !writable || parent.Err() != nil {
		return
	}
	generationErr := ctx.Err()
	fail := func(f provider.Failure) {
		if terminalAttempted {
			return
		}
		selected = true
		report = NewFailureReport(f, time.Since(started))
		_ = emit(parent, "error", struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}{"LLM request failed, please retry", "upstream_error"})
	}
	classify := func(err error) provider.Failure {
		if invalidEvents {
			return provider.NewFailure(provider.PhaseTurn, provider.EventsInvalid, 0)
		}
		if f, ok := provider.FailureOf(err); ok {
			return f
		}
		if generationErr == context.DeadlineExceeded {
			return provider.NewFailure(provider.PhaseTurn, provider.GenerationTimeout, 0)
		}
		return provider.NewFailure(provider.PhaseTurn, provider.ProviderFailure, 0)
	}
	if streamErr != nil || eventErr != nil || generationErr != nil {
		fail(classify(streamErr))
		return
	}
	batch, e := tools.ValidateBatch(calls, req.Config.Capabilities)
	if e != nil {
		fail(provider.NewFailure(provider.PhaseTurn, provider.ToolBatchInvalid, 0))
		return
	}
	seen := map[string]bool{}
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
	publicationStarted := false
	publish := func(bound context.Context, translated []tools.Call) error {
		if publicationStarted || len(translated) != len(batch.Calls) {
			invalidEvents = true
			return errEvents
		}
		publicationStarted = true
		ids := make(map[string]bool, len(translated))
		for j, c := range translated {
			if strings.TrimSpace(c.ID) == "" || ids[c.ID] || seen[c.ID] || c.Name != batch.Calls[j].Name || !bytes.Equal(c.Input, batch.Calls[j].Input) {
				invalidEvents = true
				return errEvents
			}
			ids[c.ID] = true
		}
		for _, c := range translated {
			if e := emit(bound, "tool_call", c); e != nil {
				return e
			}
		}
		if batch.Kind == tools.LocalTools {
			if e := status(bound, "waiting_for_tools"); e != nil {
				return e
			}
		}
		return emit(bound, "done", struct {
			SessionID string `json:"session_id"`
			Usage     any    `json:"usage"`
		}{req.SessionID, usagePayload(usage)})
	}
	if prepared != nil {
		e = prepared.Publish(ctx, batch, publish)
		if e != nil {
			if !writable || parent.Err() != nil || terminalAttempted {
				return
			}
			fail(classify(e))
			return
		}
		if !publicationStarted || !terminalSucceeded {
			fail(provider.NewFailure(provider.PhaseTurn, provider.EventsInvalid, 0))
			return
		}
		if terminalSucceeded {
			reasoningReport, reasoningSelected = NewReasoningReport(prepared.Outcome(), time.Since(started))
		}
	} else {
		// Preserve the legacy path's terminal context behavior; only protected
		// transaction publication uses the mandatory original-lifetime context.
		_ = publish(parent, batch.Calls)
	}
}

// Unknown counts remain null; prices are not subscription credit estimates.
func usagePayload(u provider.Usage) map[string]any {
	count := func(v int64, known bool) any {
		if known {
			return v
		}
		return nil
	}
	return map[string]any{"input_tokens": count(u.InputTokens, u.InputKnown), "output_tokens": count(u.OutputTokens, u.OutputKnown), "total_tokens": count(u.TotalTokens, u.TotalKnown), "cached_tokens": count(u.CachedTokens, u.CachedKnown), "cache_creation_tokens": nil, "input_cost": nil, "output_cost": nil, "total_cost": nil, "provider_cost": nil}
}
