package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
	"github.com/Jghh42/atuin-ai/internal/turn"
)

type diagnosticWriterFunc func([]byte) (int, error)

func (f diagnosticWriterFunc) Write(b []byte) (int, error) { return f(b) }

type deadlineRecorder struct{ *httptest.ResponseRecorder }

func (*deadlineRecorder) SetWriteDeadline(time.Time) error { return nil }

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (f diagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type observedBody struct{ closed *atomic.Int32 }

func (*observedBody) Read([]byte) (int, error) { return 0, io.EOF }
func (b *observedBody) Close() error           { b.closed.Add(1); return nil }

func TestFailureLoggerExactAllowlist(t *testing.T) {
	for _, status := range []int{0, 401, 403, 429, 503, 418} {
		var buf bytes.Buffer
		observer := NewFailureObserver(slog.New(slog.NewJSONHandler(&buf, nil)))
		observer(turn.NewFailureReport(provider.NewFailure(provider.PhaseHTTP, provider.HTTPRejected, status), 17*time.Millisecond))
		var fields map[string]any
		if json.Unmarshal(buf.Bytes(), &fields) != nil || len(fields) != 7 {
			t.Fatal("not exact seven keys", buf.String())
		}
		for _, key := range []string{"time", "level", "msg", "phase", "category", "upstream_http_status", "elapsed_ms"} {
			if _, ok := fields[key]; !ok {
				t.Fatal("missing key", key)
			}
		}
		if fields["msg"] != "generation_failed" || fields["level"] != "ERROR" || fields["elapsed_ms"] != float64(17) {
			t.Fatal(fields)
		}
		if status == 0 {
			if fields["upstream_http_status"] != nil {
				t.Fatal(fields)
			}
		} else if fields["upstream_http_status"] != float64(status) {
			t.Fatal(fields)
		}
	}
	if NewFailureObserver(nil) != nil {
		t.Fatal("nil logger")
	}
}
func TestFailureLoggerBrokenWritersPreserveWire(t *testing.T) {
	for _, panicWriter := range []bool{false, true} {
		observer := NewFailureObserver(slog.New(slog.NewJSONHandler(diagnosticWriterFunc(func([]byte) (int, error) {
			if panicWriter {
				panic("SYNTHETIC-panic")
			}
			return 0, errors.New("SYNTHETIC-writer-secret")
		}), nil)))
		r := testRuntime(t, streamProvider(func(context.Context, provider.Request, func(provider.Event) error) (provider.Usage, error) {
			return provider.Usage{}, errors.New("SYNTHETIC-provider-secret")
		}))
		r.FailureObserver = observer
		s, err := NewWithRuntime(config.Default(), nil, r)
		if err != nil {
			t.Fatal(err)
		}
		w := &deadlineRecorder{httptest.NewRecorder()}
		s.http.Handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/cli/chat", strings.NewReader(simpleBody)))
		want := "event: status\ndata: {\"state\":\"processing\"}\n\nevent: status\ndata: {\"state\":\"thinking\"}\n\nevent: error\ndata: {\"message\":\"LLM request failed, please retry\",\"code\":\"upstream_error\"}\n\n"
		if w.Body.String() != want {
			t.Fatal("wire changed", w.Body.String())
		}
	}
}

func TestBlockedTrustedLoggerHoldsAdmissionAfterTerminalAndCleanup(t *testing.T) {
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	writer := diagnosticWriterFunc(func(b []byte) (int, error) { entered <- struct{}{}; <-release; return len(b), nil })
	// All four observer entries are witnessed independently; the shared JSON
	// handler serializes their actual writes behind the first blocked writer.
	var closed atomic.Int32
	contexts := make(chan context.Context, 4)
	r := testRuntime(t, nil)
	c := codex.RequestClient{Credentials: r.Credentials, Transport: diagnosticTransport(func(req *http.Request) (*http.Response, error) {
		contexts <- req.Context()
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &observedBody{&closed}}, nil
	})}
	r.Provider = c
	shared := NewFailureObserver(slog.New(slog.NewJSONHandler(writer, nil)))
	r.FailureObserver = func(report turn.FailureReport) { entered <- struct{}{}; shared(report) }
	s, err := NewWithRuntime(config.Default(), nil, r)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var ws [4]*deadlineRecorder
	for i := range ws {
		ws[i] = &deadlineRecorder{httptest.NewRecorder()}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.http.Handler.ServeHTTP(ws[i], httptest.NewRequest("POST", "/api/cli/chat", strings.NewReader(simpleBody)))
		}(i)
	}
	// Four observer entries plus the first actual blocked JSON write.
	for range 5 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			once.Do(func() { close(release) })
			wg.Wait()
			t.Fatal("logger boundary not reached")
		}
	}
	if closed.Load() != 4 {
		t.Fatal("upstream bodies not closed before logging", closed.Load())
	}
	for range 4 {
		if (<-contexts).Err() == nil {
			t.Fatal("upstream context not canceled before logging")
		}
	}
	for _, w := range ws {
		if !strings.HasSuffix(w.Body.String(), "event: error\ndata: {\"message\":\"LLM request failed, please retry\",\"code\":\"upstream_error\"}\n\n") || !w.Flushed {
			t.Fatal("terminal not flushed before logger")
		}
	}
	extra := &deadlineRecorder{httptest.NewRecorder()}
	s.http.Handler.ServeHTTP(extra, httptest.NewRequest("POST", "/api/cli/chat", strings.NewReader(simpleBody)))
	if extra.Code != 503 || !strings.Contains(extra.Body.String(), "busy") {
		t.Fatal("blocked logger did not retain admission")
	}
	once.Do(func() { close(release) })
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handlers not joined after release")
	}
}
func TestEnabledObserverRepeatedNativeFailuresRecoverAdmission(t *testing.T) {
	beforeG := runtime.NumGoroutine()
	beforeFD, _ := os.ReadDir("/proc/self/fd")
	var requests, closed, logs atomic.Int32
	var success atomic.Bool
	r := testRuntime(t, nil)
	c := codex.RequestClient{Credentials: r.Credentials, Transport: diagnosticTransport(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		if success.Load() {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"SYNTHETIC-response\",\"status\":\"completed\",\"output\":[]}}\n\n"))}, nil
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &observedBody{&closed}}, nil
	})}
	r.Provider = c
	r.FailureObserver = NewFailureObserver(slog.New(slog.NewJSONHandler(diagnosticWriterFunc(func(b []byte) (int, error) { logs.Add(1); return len(b), nil }), nil)))
	s, err := NewWithRuntime(config.Default(), nil, r)
	if err != nil {
		t.Fatal(err)
	}
	for range 24 {
		w := &deadlineRecorder{httptest.NewRecorder()}
		s.http.Handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/cli/chat", strings.NewReader(simpleBody)))
		if !strings.Contains(w.Body.String(), "event: error") || w.Code != 200 {
			t.Fatal("failure admission not recovered")
		}
	}
	success.Store(true)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := &deadlineRecorder{httptest.NewRecorder()}
			s.http.Handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/cli/chat", strings.NewReader(simpleBody)))
			if w.Code != 200 || !strings.Contains(w.Body.String(), "event: done") {
				t.Error("success admission not recovered")
			}
		}()
	}
	wg.Wait()
	if requests.Load() != 28 || closed.Load() != 24 || logs.Load() != 24 {
		t.Fatal(requests.Load(), closed.Load(), logs.Load())
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > beforeG && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runtime.NumGoroutine() > beforeG {
		t.Fatal("goroutine retention")
	}
	afterFD, _ := os.ReadDir("/proc/self/fd")
	if len(beforeFD) > 0 && len(afterFD) > len(beforeFD) {
		t.Fatal("FD retention")
	}
}
