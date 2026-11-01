package turn

import (
	"context"
	"errors"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

type preparedFake struct {
	generation *generationFake
	rawCalls   int
}

func (p *preparedFake) Stream(context.Context, provider.Request, func(provider.Event) error) (provider.Usage, error) {
	p.rawCalls++
	return provider.Usage{}, errors.New("raw fallback forbidden")
}
func (p *preparedFake) Prepare(ctx context.Context, _ provider.Request) (provider.PreparedGeneration, error) {
	if p.generation == nil {
		return nil, nil
	}
	g := p.generation
	g.original = ctx
	g.scope, g.cancel = context.WithCancel(ctx)
	return g, nil
}

type generationFake struct {
	original, scope               context.Context
	cancel                        context.CancelFunc
	calls                         []tools.Call
	mode                          string
	nativeDone, closed, leaseHeld bool
	publishes                     int
	emitError                     error
}

func (g *generationFake) Stream(_ context.Context, sink func(provider.Event) error) (provider.Usage, error) {
	defer func() { g.nativeDone = true }()
	if len(g.calls) > 0 {
		if e := sink(provider.Event{Kind: provider.ToolCalls, Calls: g.calls}); e != nil {
			return provider.Usage{}, e
		}
	}
	return provider.Usage{}, nil
}
func (g *generationFake) Publish(_ context.Context, b tools.Batch, emit func(context.Context, []tools.Call) error) error {
	g.publishes++
	g.leaseHeld = true
	defer func() { g.leaseHeld = false }()
	if g.mode == "cancel-before-emit" {
		g.cancel()
		return provider.WithFailure(context.Canceled, provider.NewFailure(provider.PhaseContinuation, provider.ContinuationLifecycleChanged, 0))
	}
	if g.mode == "no-callback" {
		return nil
	}
	translated := append([]tools.Call(nil), b.Calls...)
	for j := range translated {
		translated[j].ID = "translated-public-id"
	}
	if g.mode == "invalid-translated" {
		translated[0].Input = []byte(`{}`)
	}
	g.emitError = emit(g.scope, translated)
	if g.emitError != nil {
		return g.emitError
	}
	if g.mode == "post-done-failure" {
		return provider.WithFailure(context.Canceled, provider.NewFailure(provider.PhaseContinuation, provider.ContinuationLifecycleChanged, 0))
	}
	if g.mode == "twice" {
		return emit(g.scope, translated)
	}
	if g.mode == "close-after-done" {
		g.cancel()
	}
	return nil
}
func (g *generationFake) Outcome() provider.GenerationOutcome {
	if g.scope.Err() != nil {
		return provider.GenerationOutcome{}
	}
	return provider.NewGenerationOutcome(true, true)
}
func (g *generationFake) Close() { g.closed = true; g.cancel() }
func TestPreparedTerminalTransactionAndDeferredReasoning(t *testing.T) {
	for _, mode := range []string{"success", "cancel-before-emit", "post-done-failure", "twice", "no-callback", "writer-tool", "writer-done", "invalid-translated", "close-after-done"} {
		t.Run(mode, func(t *testing.T) {
			g := &generationFake{mode: mode, calls: []tools.Call{call("backend", "read_file", `{"file_path":"synthetic"}`)}}
			p := &preparedFake{generation: g}
			req := atuin.Request{SessionID: "session", Config: atuin.Config{Capabilities: atuin.Capabilities{"client_v1_read_file": true}}}
			counts := map[string]int{}
			writerError := errors.New("synthetic write error")
			failures, reasoning := 0, 0
			RunWithObservers(context.Background(), p, req, SinkFunc(func(ctx context.Context, e Event) error {
				counts[e.Name]++
				if e.Name == "tool_call" || e.Name == "done" {
					if ctx != g.scope || ctx.Err() != nil {
						t.Fatal("publication switched to parent-only context")
					}
				}
				if mode == "writer-tool" && e.Name == "tool_call" || mode == "writer-done" && e.Name == "done" {
					return writerError
				}
				return nil
			}), func(FailureReport) {
				failures++
				if !g.closed || g.leaseHeld || !g.nativeDone || g.original.Err() == nil {
					t.Fatal("failure observer ran before transaction/native cleanup/unlock/cancel")
				}
			}, func(r ReasoningReport) {
				reasoning++
				if !g.closed || g.leaseHeld || !g.nativeDone || g.original.Err() == nil {
					t.Fatal("reasoning observer ran before complete cleanup")
				}
				if r.Message() != "generation_reasoning_replayed" || r.Category() != "reasoning_replayed_and_handled" {
					t.Fatal("wrong closed report")
				}
			})
			if p.rawCalls != 0 {
				t.Fatal("prepared provider silently fell back to raw Stream")
			}
			switch mode {
			case "success":
				if counts["tool_call"] != 1 || counts["done"] != 1 || counts["error"] != 0 || reasoning != 1 || failures != 0 {
					t.Fatal("successful transaction lost terminal/report")
				}
			case "cancel-before-emit", "no-callback", "invalid-translated":
				if counts["tool_call"] != 0 || counts["done"] != 0 || counts["error"] != 1 || reasoning != 0 || failures != 1 {
					t.Fatal("failed publication released tools or lost generic terminal")
				}
			case "post-done-failure", "twice", "close-after-done":
				if counts["done"] != 1 || counts["error"] != 0 || counts["tool_call"] != 1 || reasoning != 0 || failures != 0 {
					t.Fatal("post-terminal state caused another frame/report")
				}
			case "writer-tool", "writer-done":
				if counts["error"] != 0 || reasoning != 0 || failures != 0 || g.emitError != writerError {
					t.Fatal("writer error identity lost or extra error/report emitted")
				}
			}
		})
	}
}
func TestPreparedValidationAndNilPrepareFailClosed(t *testing.T) {
	req := atuin.Request{SessionID: "session"}
	p := &preparedFake{}
	names := map[string]int{}
	Run(context.Background(), p, req, SinkFunc(func(_ context.Context, e Event) error { names[e.Name]++; return nil }))
	if p.rawCalls != 0 || names["error"] != 1 || names["done"] != 0 {
		t.Fatal("nil successful Prepare bypassed transaction")
	}
	g := &generationFake{calls: []tools.Call{call("bad", "unknown_hosted", `{}`)}}
	p.generation = g
	names = map[string]int{}
	Run(context.Background(), p, req, SinkFunc(func(_ context.Context, e Event) error { names[e.Name]++; return nil }))
	if g.publishes != 0 || names["tool_call"] != 0 || names["error"] != 1 {
		t.Fatal("batch validation did not precede publication")
	}
}
