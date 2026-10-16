package httpserver

import (
	"bufio"
	"context"
	"errors"
	"github.com/Jghh42/atuin-ai/internal/atuin"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

func TestFlushBeforeCompletionAndFailureWithheldCalls(t *testing.T) {
	release := make(chan struct{})
	ts := runtimeServer(t, testRuntime(t, streamProvider(func(ctx context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
		if err := s(provider.Event{Kind: provider.TextDelta, Text: "visible"}); err != nil {
			return provider.Usage{}, err
		}
		_ = s(provider.Event{Kind: provider.ToolCalls, Calls: []tools.Call{{ID: "a", Name: "read_file", Input: []byte(`{"file_path":"private"}`)}}})
		select {
		case <-release:
		case <-ctx.Done():
			return provider.Usage{}, ctx.Err()
		}
		return provider.Usage{}, errors.New("private provider failure")
	})))
	resp, err := http.Post(ts.URL+"/api/cli/chat", "application/json", strings.NewReader(simpleBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	frames := make(chan string, 1)
	go func() {
		var b strings.Builder
		for i := 0; i < 9; i++ {
			line, err := reader.ReadString('\n')
			if err != nil {
				frames <- "read failed"
				return
			}
			b.WriteString(line)
		}
		frames <- b.String()
	}()
	select {
	case got := <-frames:
		if !strings.Contains(got, "event: text\ndata: {\"content\":\"visible\"}") {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("text was not flushed")
	}
	close(release)
	b, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "event: error\ndata: {\"message\":\"LLM request failed, please retry\",\"code\":\"upstream_error\"}\n\n" {
		t.Fatal(string(b))
	}
}

// ResponseController must observe both write and flush errors; neither path may
// be followed by another write, even a terminal-error retry.
type failingWriter struct {
	header          http.Header
	writes, flushes int
	failFlush       bool
}

func (w *failingWriter) Header() http.Header              { return w.header }
func (w *failingWriter) WriteHeader(int)                  {}
func (w *failingWriter) SetWriteDeadline(time.Time) error { return nil }
func (w *failingWriter) Write(b []byte) (int, error) {
	w.writes++
	if !w.failFlush {
		return 0, errors.New("private write")
	}
	return len(b), nil
}
func (w *failingWriter) FlushError() error { w.flushes++; return errors.New("private flush") }
func TestHTTPWriteAndFlushFailuresNoRetry(t *testing.T) {
	for _, flush := range []bool{false, true} {
		generations := 0
		r := testRuntime(t, streamProvider(func(context.Context, provider.Request, func(provider.Event) error) (provider.Usage, error) {
			generations++
			return provider.Usage{}, nil
		}))
		req := httptest.NewRequest("POST", "/api/cli/chat", strings.NewReader(simpleBody))
		normalized, err := (atuin.Decoder{Catalog: atuin.ModelCatalog{Default: "model", Models: map[string]string{"model": "model"}}}).Decode([]byte(simpleBody))
		if err != nil {
			t.Fatal(err)
		}
		w := &failingWriter{header: make(http.Header), failFlush: flush}
		streamChat(w, req, r, normalized)
		if w.writes != 1 || generations != 0 || w.flushes != boolInt(flush) {
			t.Fatal(w, generations)
		}
	}
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
