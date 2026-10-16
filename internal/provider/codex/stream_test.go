package codex

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider"
)

func TestStreamTCPClosingAndBounds(t *testing.T) {
	sentinel := errors.New("downstream failed")
	for _, mode := range []string{"success", "nonstandard MIME", "header stall", "body stall", "heartbeat generation bound", "cancel", "sink", "EOF", "failed"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			upstreamClosed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				defer close(upstreamClosed)
				_, _ = io.Copy(io.Discard, r.Body)
				if mode == "header stall" {
					<-r.Context().Done()
					return
				}
				if mode == "nonstandard MIME" {
					w.Header().Set("Content-Type", "application/octet-stream")
				} else {
					w.Header().Set("Content-Type", "text/event-stream")
				}
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				switch mode {
				case "EOF":
					return
				case "failed":
					_, _ = io.WriteString(w, frame("response.failed", nil))
					w.(http.Flusher).Flush()
					return
				case "success", "nonstandard MIME":
					_, _ = io.WriteString(w, complete(fn("item", "call", "{}")))
					w.(http.Flusher).Flush()
				case "sink":
					_, _ = io.WriteString(w, added(0, msg(""))+textDelta("hi"))
					w.(http.Flusher).Flush()
				case "heartbeat generation bound":
					timer := time.NewTicker(5 * time.Millisecond)
					defer timer.Stop()
					for {
						select {
						case <-r.Context().Done():
							return
						case <-timer.C:
							_, _ = io.WriteString(w, ": ping\n\n")
							w.(http.Flusher).Flush()
						}
					}
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			client := RequestClient{URL: server.URL, Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return credential(), nil })}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				go func() { time.Sleep(20 * time.Millisecond); cancel() }()
			}
			var events []provider.Event
			generation := time.Second
			if mode == "heartbeat generation bound" {
				generation = 80 * time.Millisecond
			}
			usage, err := client.stream(ctx, decodedRequest(t, `[{"role":"user","content":"hello"}]`), func(e provider.Event) error {
				events = append(events, e)
				if mode == "sink" {
					return sentinel
				}
				return nil
			}, generation, 60*time.Millisecond)
			_ = usage
			switch mode {
			case "success", "nonstandard MIME":
				if err != nil || len(events) != 1 || events[0].Kind != provider.ToolCalls {
					t.Fatalf("%v %v", events, err)
				}
			case "sink":
				if !errors.Is(err, sentinel) {
					t.Fatal(err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case "header stall", "body stall", "heartbeat generation bound":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
			default:
				if err == nil {
					t.Fatal("success on failure")
				}
			}
			if requests.Load() != 1 {
				t.Fatal("retried")
			}
			select {
			case <-upstreamClosed:
			case <-time.After(time.Second):
				t.Fatal("upstream not closed promptly")
			}
		})
	}
}

type closeReader struct {
	closed   chan struct{}
	started  chan struct{}
	once     sync.Once
	readOnce sync.Once
	reads    atomic.Int32
}

func (r *closeReader) Read([]byte) (int, error) {
	r.reads.Add(1)
	r.readOnce.Do(func() { close(r.started) })
	<-r.closed
	return 0, errors.New("private transport read detail")
}
func (r *closeReader) Close() error { r.once.Do(func() { close(r.closed) }); return nil }

type streamTransportFunc func(*http.Request) (*http.Response, error)

func (f streamTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestBlockedReadClosedOnTimeoutAndCancel(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"hello"}]`)
	// Handshake on an ACTUAL blocked Read, not a sleep racing projection/Open.
	// Real inactivity/header/generation timers remain covered by the TCP test.
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			body := &closeReader{closed: make(chan struct{}), started: make(chan struct{})}
			c := RequestClient{Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return credential(), nil }), Transport: streamTransportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
			})}
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			done := make(chan error, 1)
			go func() {
				_, e := c.stream(ctx, req, func(provider.Event) error { return nil }, time.Hour, time.Hour)
				done <- e
			}()
			select {
			case <-body.started:
			case e := <-done:
				t.Fatal("stream returned before blocked read", e)
			case <-time.After(2 * time.Second):
				t.Fatal("read did not start")
			}
			cancel(cause)
			select {
			case e := <-done:
				if !errors.Is(e, cause) {
					t.Fatal("wrong cancellation cause", e)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("blocked reader not joined")
			}
			select {
			case <-body.closed:
			default:
				t.Fatal("acquired reader left blocked")
			}
		})
	}
}
func TestCancellationBeforeOpenDoesNotOwnUnacquiredBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &closeReader{closed: make(chan struct{}), started: make(chan struct{})}
	var opened atomic.Bool
	c := RequestClient{Now: func() time.Time { cancel(); return projectionTime }, Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return credential(), nil }), Transport: streamTransportFunc(func(*http.Request) (*http.Response, error) {
		opened.Store(true)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
	})}
	_, err := c.Stream(ctx, decodedRequest(t, `[{"role":"user","content":"hello"}]`), func(provider.Event) error { return nil })
	if !errors.Is(err, context.Canceled) || opened.Load() || body.reads.Load() != 0 {
		t.Fatal("pre-open cancellation did not stop transport", err)
	}
	select {
	case <-body.closed:
		t.Fatal("closed a body never acquired from transport")
	default:
	}
}
func TestStreamCredentialAndOpenCancellation(t *testing.T) {
	for _, stage := range []string{"credentials", "open"} {
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		c := RequestClient{Credentials: credentials.SourceFunc(func(ctx context.Context) (credentials.Credential, error) {
			if stage == "credentials" {
				close(started)
				<-ctx.Done()
				return credentials.Credential{}, errors.New("private")
			}
			return credential(), nil
		}), Transport: streamTransportFunc(func(r *http.Request) (*http.Response, error) {
			close(started)
			<-r.Context().Done()
			return nil, errors.New("private")
		})}
		go func() { <-started; cancel() }()
		_, err := c.Stream(ctx, decodedRequest(t, `[{"role":"user","content":"hello"}]`), func(provider.Event) error { return nil })
		cancel()
		if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private") {
			t.Fatal(stage, err)
		}
	}
}
func TestStreamReadErrorsSafe(t *testing.T) {
	c := RequestClient{Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return credential(), nil }), Transport: streamTransportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(errorReader{})}, nil
	})}
	_, err := c.Stream(context.Background(), decodedRequest(t, `[{"role":"user","content":"hello"}]`), func(provider.Event) error { return nil })
	if !errors.Is(err, ErrStream) || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("secret") }
