package turn

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

type fakeProvider func(context.Context, provider.Request, func(provider.Event) error) (provider.Usage, error)

func (f fakeProvider) Stream(ctx context.Context, r provider.Request, s func(provider.Event) error) (provider.Usage, error) {
	return f(ctx, r, s)
}
func call(id, name, input string) tools.Call {
	return tools.Call{ID: id, Name: name, Input: json.RawMessage(input)}
}

func TestSequenceGoldens(t *testing.T) {
	local := call("a", "read_file", `{"file_path":"a"}`)
	suggestion := call("s", "suggest_command", `{"command":"rm -rf /","description":"x","confidence":"high","danger":"low"}`)
	for _, tc := range []struct {
		name  string
		calls []tools.Call
		bad   bool
		want  []string
	}{
		{"text", nil, false, []string{"status:processing", "status:thinking", "text", "done"}},
		{"parallel", []tools.Call{local, call("b", "write_file", `{"file_path":"b","content":"x"}`)}, false, []string{"status:processing", "status:thinking", "text", "tool_call:a", "tool_call:b", "status:waiting_for_tools", "done"}},
		{"suggestion", []tools.Call{suggestion}, false, []string{"status:processing", "status:thinking", "text", "tool_call:s", "done"}},
		{"failed after callback", []tools.Call{local}, true, []string{"status:processing", "status:thinking", "text", "error"}},
		{"mixed", []tools.Call{suggestion, local}, false, []string{"status:processing", "status:thinking", "text", "error"}},
		{"disabled", []tools.Call{call("x", "edit_file", `{"file_path":"x","old_string":"a","new_string":"b"}`)}, false, []string{"status:processing", "status:thinking", "text", "error"}},
		{"unknown", []tools.Call{call("x", "web_search", `{}`)}, false, []string{"status:processing", "status:thinking", "text", "error"}},
		{"schema", []tools.Call{call("x", "read_file", `{}`)}, false, []string{"status:processing", "status:thinking", "text", "error"}},
		{"collision", []tools.Call{local, local}, false, []string{"status:processing", "status:thinking", "text", "error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := atuin.Request{SessionID: "session", Config: atuin.Config{Capabilities: atuin.Capabilities{"client_v1_read_file": true, "client_v1_write_file": true}}}
			var got []string
			calls := 0
			p := fakeProvider(func(ctx context.Context, r provider.Request, s func(provider.Event) error) (provider.Usage, error) {
				calls++
				if err := s(provider.Event{Kind: provider.TextDelta, Text: "hello\nworld"}); err != nil {
					return provider.Usage{}, err
				}
				if tc.calls != nil {
					if err := s(provider.Event{Kind: provider.ToolCalls, Calls: tc.calls}); err != nil {
						return provider.Usage{}, err
					}
				}
				// No tool may escape before return, even if this provider later fails.
				if len(got) != 3 {
					t.Fatal(got)
				}
				if tc.bad {
					return provider.Usage{}, errors.New("secret")
				}
				return provider.Usage{}, nil
			})
			Run(context.Background(), p, req, SinkFunc(func(_ context.Context, e Event) error {
				name := e.Name
				b, _ := json.Marshal(e.Data)
				if e.Name == "status" {
					var v struct{ State string }
					_ = json.Unmarshal(b, &v)
					name += ":" + v.State
				}
				if e.Name == "tool_call" {
					c := e.Data.(tools.Call)
					name += ":" + c.ID
					if c.Name == "suggest_command" {
						var v struct{ Danger string }
						_ = json.Unmarshal(c.Input, &v)
						if v.Danger != "high" {
							t.Fatal(string(c.Input))
						}
					}
				}
				got = append(got, name)
				return nil
			}))
			if calls != 1 || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%d %v want %v", calls, got, tc.want)
			}
		})
	}
}

func TestMalformedDuplicateAndLateEvents(t *testing.T) {
	for _, kind := range []string{"duplicate", "text_after_batch", "unknown", "history_collision"} {
		t.Run(kind, func(t *testing.T) {
			var late func(provider.Event) error
			var names []string
			req := atuin.Request{Config: atuin.Config{Capabilities: atuin.Capabilities{"client_v1_read_file": true}}}
			if kind == "history_collision" {
				req.Messages = []atuin.Message{{Content: []atuin.Block{{Type: "tool_use", ID: "a"}}}}
			}
			p := fakeProvider(func(ctx context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
				late = s
				_ = s(provider.Event{Kind: provider.ToolCalls, Calls: []tools.Call{call("a", "read_file", `{"file_path":"x"}`)}})
				switch kind {
				case "duplicate":
					_ = s(provider.Event{Kind: provider.ToolCalls})
				case "text_after_batch":
					_ = s(provider.Event{Kind: provider.TextDelta})
				case "unknown":
					_ = s(provider.Event{Kind: "bad"})
				}
				return provider.Usage{}, nil
			})
			Run(context.Background(), p, req, SinkFunc(func(_ context.Context, e Event) error { names = append(names, e.Name); return nil }))
			if !reflect.DeepEqual(names, []string{"status", "status", "error"}) {
				t.Fatal(names)
			}
			if late(provider.Event{Kind: provider.TextDelta}) == nil {
				t.Fatal("late event accepted")
			}
			if len(names) != 3 {
				t.Fatal(names)
			}
		})
	}
}

func TestSinkFailureCancelsWithoutRetry(t *testing.T) {
	for failAt := 1; failAt <= 6; failAt++ {
		n, generations := 0, 0
		var late func(provider.Event) error
		var generationCtx context.Context
		p := fakeProvider(func(ctx context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
			generations++
			late = s
			generationCtx = ctx
			err := s(provider.Event{Kind: provider.TextDelta, Text: "x"})
			if err != nil && ctx.Err() == nil {
				t.Fatal("not canceled")
			}
			if err == nil {
				err = s(provider.Event{Kind: provider.ToolCalls, Calls: []tools.Call{call("a", "read_file", `{"file_path":"x"}`)}})
			}
			return provider.Usage{}, err
		})
		Run(context.Background(), p, atuin.Request{Config: atuin.Config{Capabilities: atuin.Capabilities{"client_v1_read_file": true}}}, SinkFunc(func(_ context.Context, e Event) error {
			n++
			if n == failAt {
				return errors.New("secret write")
			}
			return nil
		}))
		if n != failAt {
			t.Fatalf("%d: %d", failAt, n)
		}
		if failAt <= 2 && generations != 0 {
			t.Fatal(generations)
		}
		if failAt > 2 {
			if generations != 1 || generationCtx.Err() == nil {
				t.Fatal(generations, "generation not canceled")
			}
			if late(provider.Event{Kind: provider.TextDelta, Text: "late"}) == nil || late(provider.Event{Kind: provider.ToolCalls}) == nil {
				t.Fatal("late callback accepted after sink failure")
			}
			if n != failAt {
				t.Fatal("write after sink failure", n)
			}
		}
	}
}

func TestLateCallbacksAfterSuccess(t *testing.T) {
	var late func(provider.Event) error
	var names []string
	generations := 0
	p := fakeProvider(func(_ context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
		generations++
		late = s
		return provider.Usage{}, nil
	})
	Run(context.Background(), p, atuin.Request{}, SinkFunc(func(_ context.Context, e Event) error { names = append(names, e.Name); return nil }))
	if generations != 1 || !reflect.DeepEqual(names, []string{"status", "status", "done"}) {
		t.Fatal(generations, names)
	}
	for _, event := range []provider.Event{{Kind: provider.TextDelta, Text: "late"}, {Kind: provider.ToolCalls, Calls: []tools.Call{call("a", "read_file", `{"file_path":"x"}`)}}} {
		if late(event) == nil {
			t.Fatal("late callback accepted after success")
		}
	}
	if !reflect.DeepEqual(names, []string{"status", "status", "done"}) {
		t.Fatal("writes after terminal", names)
	}
}

func TestTerminalErrorWriteFailureNoConsequentWrites(t *testing.T) {
	generations, writes := 0, 0
	var late func(provider.Event) error
	var generationCtx context.Context
	var names []string
	p := fakeProvider(func(ctx context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
		generations++
		late = s
		generationCtx = ctx
		if err := s(provider.Event{Kind: provider.TextDelta, Text: "visible"}); err != nil {
			t.Fatal(err)
		}
		return provider.Usage{}, errors.New("private upstream failure")
	})
	Run(context.Background(), p, atuin.Request{}, SinkFunc(func(_ context.Context, e Event) error {
		writes++
		names = append(names, e.Name)
		if e.Name == "error" {
			b, _ := json.Marshal(e.Data)
			if string(b) != `{"message":"LLM request failed, please retry","code":"upstream_error"}` {
				t.Fatal(string(b))
			}
			return errors.New("private terminal write failure")
		}
		return nil
	}))
	if generations != 1 || writes != 4 || generationCtx.Err() == nil || !reflect.DeepEqual(names, []string{"status", "status", "text", "error"}) {
		t.Fatal(generations, writes, names)
	}
	if late(provider.Event{Kind: provider.TextDelta}) == nil || writes != 4 {
		t.Fatal("consequent write after terminal sink failure")
	}
}

func TestGenerationTimeoutIsTerminalEvenIfProviderReturnsSuccess(t *testing.T) {
	var names []string
	p := fakeProvider(func(ctx context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
		_ = s(provider.Event{Kind: provider.ToolCalls, Calls: []tools.Call{call("a", "read_file", `{"file_path":"x"}`)}})
		<-ctx.Done()
		return provider.Usage{}, nil
	})
	run(context.Background(), p, atuin.Request{Config: atuin.Config{Capabilities: atuin.Capabilities{"client_v1_read_file": true}}}, SinkFunc(func(_ context.Context, e Event) error { names = append(names, e.Name); return nil }), time.Millisecond)
	if !reflect.DeepEqual(names, []string{"status", "status", "error"}) {
		t.Fatal(names)
	}
}

func TestUsageUnknownAndZero(t *testing.T) {
	b, _ := json.Marshal(usagePayload(provider.Usage{InputKnown: true, OutputTokens: 3, OutputKnown: true, TotalTokens: 3, TotalKnown: true, CachedKnown: true}))
	want := `{"cache_creation_tokens":null,"cached_tokens":0,"input_cost":null,"input_tokens":0,"output_cost":null,"output_tokens":3,"provider_cost":null,"total_cost":null,"total_tokens":3}`
	if string(b) != want {
		t.Fatal(string(b))
	}
}
