package httpserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/turn"
)

func TestReasoningObserverSevenClosedKeys(t *testing.T) {
	for _, flags := range []provider.GenerationOutcome{provider.NewGenerationOutcome(true, false), provider.NewGenerationOutcome(false, true), provider.NewGenerationOutcome(true, true)} {
		report, ok := turn.NewReasoningReport(flags, -time.Second)
		if !ok {
			t.Fatal("closed report missing")
		}
		var out bytes.Buffer
		observer := NewReasoningObserver(slog.New(slog.NewJSONHandler(&out, nil)))
		observer(report)
		var v map[string]any
		if json.Unmarshal(out.Bytes(), &v) != nil || len(v) != 7 {
			t.Fatal("diagnostic shape changed")
		}
		for _, key := range []string{"time", "level", "msg", "phase", "category", "upstream_http_status", "elapsed_ms"} {
			if _, ok := v[key]; !ok {
				t.Fatal("missing closed diagnostic key")
			}
		}
		if v["msg"] != report.Message() || v["phase"] != "stream" || v["category"] != report.Category() || v["upstream_http_status"] != nil || v["elapsed_ms"] != float64(0) {
			t.Fatal("unsafe report values")
		}
	}
	if _, ok := turn.NewReasoningReport(provider.GenerationOutcome{}, time.Second); ok {
		t.Fatal("ordinary success reported reasoning")
	}
	var out bytes.Buffer
	NewReasoningObserver(slog.New(slog.NewJSONHandler(&out, nil)))(turn.ReasoningReport{})
	if out.Len() != 0 {
		t.Fatal("unknown outcome logged as success")
	}
}

type reasoningBlockedWriter struct{ entered, release chan struct{} }

func (w reasoningBlockedWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}
func TestReasoningObserverTrustedSynchronousWriterBoundary(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	writer := reasoningBlockedWriter{entered, release}
	observer := NewReasoningObserver(slog.New(slog.NewJSONHandler(writer, nil)))
	report, _ := turn.NewReasoningReport(provider.NewGenerationOutcome(true, false), time.Second)
	done := make(chan struct{})
	go func() { observer(report); close(done) }()
	<-entered
	select {
	case <-done:
		t.Fatal("observer silently detached or dropped trusted log")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("released writer did not finish observer")
	}
}
