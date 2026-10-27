package turn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

type privateFailure struct{}

func (privateFailure) Error() string { panic("SYNTHETIC-SECRET") }
func TestObservedFailurePrecedenceCleanupAndWire(t *testing.T) {
	for _, mode := range []string{"typed", "fallback", "events", "batch", "collision", "deadline", "typed-deadline", "terminal-write-fails", "observer-panics", "success", "client-cancel", "sink-fails"} {
		t.Run(mode, func(t *testing.T) {
			req := atuin.Request{SessionID: "SYNTHETIC-session", Config: atuin.Config{Capabilities: atuin.Capabilities{"client_v1_read_file": true}}}
			if mode == "collision" {
				req.Messages = []atuin.Message{{Content: []atuin.Block{{Type: "tool_use", ID: "SYNTHETIC-id"}}}}
			}
			var late func(provider.Event) error
			var upstream context.Context
			clean := false
			var names []string
			reports := 0
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := fakeProvider(func(ctx context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
				upstream, late = ctx, s
				defer func() { clean = true }()
				switch mode {
				case "success":
					return provider.Usage{}, nil
				case "client-cancel":
					cancel()
					return provider.Usage{}, privateFailure{}
				case "sink-fails":
					return provider.Usage{}, s(provider.Event{Kind: provider.TextDelta, Text: "SYNTHETIC"})
				case "events":
					_ = s(provider.Event{Kind: "SYNTHETIC-invalid"})
					if s(provider.Event{Kind: provider.TextDelta}) != errEvents {
						t.Fatal("not sticky")
					}
				case "batch", "collision":
					name, input := "SYNTHETIC-tool", `{"SYNTHETIC-input":true}`
					if mode == "collision" {
						name, input = "read_file", `{"file_path":"SYNTHETIC-path"}`
					}
					_ = s(provider.Event{Kind: provider.ToolCalls, Calls: []tools.Call{call("SYNTHETIC-id", name, input)}})
					return provider.Usage{}, nil
				case "deadline":
					<-ctx.Done()
					return provider.Usage{}, nil
				case "typed-deadline":
					<-ctx.Done()
				case "fallback":
					return provider.Usage{}, privateFailure{}
				}
				return provider.Usage{}, provider.WithFailure(errors.New("fixed"), provider.NewFailure(provider.PhaseHTTP, provider.HTTPUnauthorized, 401))
			})
			want := provider.HTTPUnauthorized
			switch mode {
			case "fallback":
				want = provider.ProviderFailure
			case "events":
				want = provider.EventsInvalid
			case "batch":
				want = provider.ToolBatchInvalid
			case "collision":
				want = provider.HistoryCallIDCollision
			case "deadline":
				want = provider.GenerationTimeout
			}
			timeout := time.Hour
			if mode == "deadline" || mode == "typed-deadline" {
				timeout = time.Millisecond
			}
			runObserved(parent, p, req, SinkFunc(func(_ context.Context, e Event) error {
				names = append(names, e.Name)
				if e.Name == "error" {
					b, _ := json.Marshal(e.Data)
					if string(b) != `{"message":"LLM request failed, please retry","code":"upstream_error"}` {
						t.Fatal("wire changed")
					}
					if mode == "terminal-write-fails" {
						return privateFailure{}
					}
				}
				if mode == "sink-fails" && e.Name == "text" {
					return privateFailure{}
				}
				return nil
			}), timeout, func(r FailureReport) {
				reports++
				if !clean || upstream.Err() == nil || len(names) == 0 || names[len(names)-1] != "error" {
					t.Fatal("report before terminal/cleanup")
				}
				// This callback acquires the runner mutex: observer MUST be outside it.
				done := make(chan error, 1)
				go func() { done <- late(provider.Event{Kind: provider.TextDelta, Text: "SYNTHETIC-late"}) }()
				select {
				case err := <-done:
					if err == nil {
						t.Fatal("late event accepted")
					}
				case <-time.After(time.Second):
					t.Fatal("observer holds callback mutex")
				}
				if r.Failure().Category() != want || r.ElapsedMS() < 0 {
					t.Fatal(r)
				}
				if mode == "observer-panics" {
					panic("SYNTHETIC-panic")
				}
			})
			if mode == "success" || mode == "client-cancel" || mode == "sink-fails" {
				if reports != 0 {
					t.Fatal("unexpected report")
				}
			} else if reports != 1 || !reflect.DeepEqual(names, []string{"status", "status", "error"}) {
				t.Fatal(reports, names)
			}
		})
	}
}
func TestValidTextAtGenerationDeadlineIsNotInvalidEvent(t *testing.T) {
	generations, reports := 0, 0
	var names []string
	p := fakeProvider(func(ctx context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
		generations++
		<-ctx.Done()
		first := s(provider.Event{Kind: provider.TextDelta, Text: "SYNTHETIC-valid-text"})
		if first == nil || s(provider.Event{Kind: provider.TextDelta, Text: "SYNTHETIC-sticky"}) != first {
			t.Fatal("callback error not sticky")
		}
		return provider.Usage{}, first
	})
	runObserved(context.Background(), p, atuin.Request{}, SinkFunc(func(_ context.Context, e Event) error {
		names = append(names, e.Name)
		if e.Name == "error" {
			b, _ := json.Marshal(e.Data)
			if string(b) != `{"message":"LLM request failed, please retry","code":"upstream_error"}` {
				t.Fatal("wire changed")
			}
		}
		return nil
	}), time.Millisecond, func(r FailureReport) {
		reports++
		if r.Failure().Category() != provider.GenerationTimeout {
			t.Errorf("valid deadline callback mislabeled: %v", r)
		}
	})
	if generations != 1 || reports != 1 || !reflect.DeepEqual(names, []string{"status", "status", "error"}) {
		t.Fatal(generations, reports, names)
	}
}

type reportPresentationCause struct{ Secret string }

func (e reportPresentationCause) Error() string  { panic(e.Secret) }
func (e reportPresentationCause) String() string { panic(e.Secret) }

func TestFailureReportSafeCompositePresentations(t *testing.T) {
	const marker = "SYNTHETIC_REPORT_TOKEN_42_PRIVATE"
	forbidden := regexp.MustCompile(`\b` + regexp.QuoteMeta(marker) + `\b`)
	for _, f := range []provider.Failure{
		provider.NewFailure(provider.PhaseHTTP, provider.HTTPRateLimited, 429),
		provider.NewFailure(provider.Phase(255), provider.Category(255), 999),
	} {
		wrapped := provider.WithFailure(reportPresentationCause{marker}, f)
		snapshot, ok := provider.FailureOf(wrapped)
		if !ok {
			t.Fatal("typed snapshot lost")
		}
		report := NewFailureReport(snapshot, 42*time.Millisecond)
		if report.Failure() != f || report.ElapsedMS() != 42 {
			t.Fatal("immutable report accessors")
		}
		composite := struct {
			Report  FailureReport
			Wrapped error
		}{report, wrapped}
		for _, value := range []any{report, &report, []any{report, wrapped}, map[string]any{"report": report, "failure": wrapped}, composite, &composite} {
			for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%X", "%b", "%e", "%f", "%c", "%U", "%t", "%o", "%z", "%T", "%p", "%w"} {
				rendered := fmt.Sprintf(verb, value)
				if forbidden.MatchString(rendered) || strings.Contains(rendered, "PANIC") {
					t.Fatalf("unsafe presentation verb %s", verb)
				}
			}
			encoded, err := json.Marshal(value)
			if err != nil || forbidden.Match(encoded) {
				t.Fatal("unsafe JSON")
			}
			var buf bytes.Buffer
			slog.New(slog.NewJSONHandler(&buf, nil)).Info("safe", "report", value)
			if forbidden.Match(buf.Bytes()) || strings.Contains(buf.String(), "PANIC") {
				t.Fatal("unsafe slog")
			}
		}
		encoded, _ := json.Marshal(report)
		if !bytes.Contains(encoded, []byte(`"elapsed_ms":42`)) {
			t.Fatal("elapsed serialization")
		}
		if f.HTTPStatus() == 0 && (!bytes.Contains(encoded, []byte(`"phase":"unknown"`)) || !bytes.Contains(encoded, []byte(`"category":"unknown"`)) || !bytes.Contains(encoded, []byte(`"upstream_http_status":null`))) {
			t.Fatal("closed fallback presentation")
		}
	}
}

func TestFailureElapsedExcludesTerminalLatency(t *testing.T) {
	var flushStarted time.Time
	var report FailureReport
	started := time.Now()
	p := fakeProvider(func(context.Context, provider.Request, func(provider.Event) error) (provider.Usage, error) {
		return provider.Usage{}, privateFailure{}
	})
	RunWithObserver(context.Background(), p, atuin.Request{}, SinkFunc(func(_ context.Context, e Event) error {
		if e.Name == "error" {
			flushStarted = time.Now()
			time.Sleep(20 * time.Millisecond)
		}
		return nil
	}), func(r FailureReport) { report = r })
	if report.ElapsedMS() > flushStarted.Sub(started).Milliseconds() {
		t.Fatal("elapsed includes terminal flush")
	}
	if report.ElapsedMS() >= time.Since(started).Milliseconds()-10 {
		t.Fatal("terminal latency included")
	}
	if NewFailureReport(provider.Failure{}, -time.Hour).ElapsedMS() != 0 {
		t.Fatal("negative duration")
	}
}

func TestNilObserverEquivalent(t *testing.T) {
	for _, fail := range []bool{false, true} {
		p := fakeProvider(func(context.Context, provider.Request, func(provider.Event) error) (provider.Usage, error) {
			if fail {
				return provider.Usage{}, privateFailure{}
			}
			return provider.Usage{}, nil
		})
		var old, newWire []Event
		Run(context.Background(), p, atuin.Request{}, SinkFunc(func(_ context.Context, e Event) error { old = append(old, e); return nil }))
		RunWithObserver(context.Background(), p, atuin.Request{}, SinkFunc(func(_ context.Context, e Event) error { newWire = append(newWire, e); return nil }), nil)
		if !reflect.DeepEqual(old, newWire) {
			t.Fatal("nil changed wire")
		}
	}
}
