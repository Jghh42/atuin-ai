package httpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

// Couple both real TCP legs: Atuin disconnect/write timeout must cancel the
// native Codex HTTP request, not merely a stand-in Provider implementation.
func TestNativeCancellationChainReleasesAdmission(t *testing.T) {
	for _, mode := range []string{"disconnect", "nonreading"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			var current atomic.Pointer[nativeCancelCycle]
			upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				c := current.Load()
				if c == nil {
					fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp\",\"status\":\"completed\",\"output\":[]}}\n\n")
					return
				}
				defer close(c.upstreamDone)
				// Observe request cancellation independently of the fake writer, which
				// might still be blocked. Handler return is NOT cancellation evidence.
				var canceled sync.Once
				reportCancellation := func() {
					canceled.Do(func() {
						now := time.Now()
						c.add("upstream request context canceled")
						c.upstreamCanceled <- now
					})
				}
				stop := context.AfterFunc(r.Context(), reportCancellation)
				defer func() {
					// Done can close before AfterFunc's goroutine starts. Do not
					// suppress that observation by stopping it on handler exit.
					if r.Context().Err() != nil {
						reportCancellation()
					}
					stop()
				}()
				controller := http.NewResponseController(w)
				if err := controller.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
					t.Error(err)
					return
				}
				fmt.Fprint(w, "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"msg\",\"type\":\"message\",\"content\":[]}}\n\n")
				if err := controller.Flush(); err != nil {
					t.Error(err)
					return
				}
				close(c.started)
				// Do not deliver text until the client has consumed headers and will
				// perform no further reads. This excludes bufio read-ahead as a drain.
				select {
				case <-c.release:
				case <-r.Context().Done():
					return
				case <-time.After(2 * time.Second):
					t.Error("client did not release native text generation")
					return
				}
				text, frames := "visible", 1
				if mode == "nonreading" {
					text, frames = strings.Repeat("x", 64<<10), 4
				}
				b, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "item_id": "msg", "delta": text})
				// Finite source, not a throughput race to fill megabytes of default
				// kernel buffering. Both TCP legs have bounded send/receive buffers.
				for range frames {
					if _, err := fmt.Fprintf(w, "event: response.output_text.delta\ndata: %s\n\n", b); err != nil {
						break
					}
					if err := controller.Flush(); err != nil {
						break
					}
				}
				select {
				case <-r.Context().Done():
				case <-time.After(2 * time.Second):
					t.Error("native request did not close promptly (fake upstream watchdog)")
				}
			}))
			upstream.Listener = nativeCancelListener{Listener: upstream.Listener}
			upstream.Start()
			defer upstream.Close()
			transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
				if err != nil {
					return nil, err
				}
				if err := conn.(*net.TCPConn).SetReadBuffer(64 << 10); err != nil {
					_ = conn.Close()
					return nil, err
				}
				return conn, nil
			}}
			defer transport.CloseIdleConnections()
			runtime := testRuntime(t, nil)
			runtime.Provider = codex.RequestClient{Credentials: runtime.Credentials, URL: upstream.URL, Transport: transport}
			runtime.WriteTimeout = 30 * time.Millisecond
			s, err := NewWithRuntime(config.Default(), nil, runtime)
			if err != nil {
				t.Fatal(err)
			}
			downstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c := current.Load()
				if c == nil {
					s.http.Handler.ServeHTTP(w, r)
					return
				}
				defer close(c.downstreamDone) // after admission's deferred slot release
				s.http.Handler.ServeHTTP(nativeTraceWriter{ResponseWriter: w, cycle: c}, r)
			}))
			downstream.Listener = nativeCancelListener{Listener: downstream.Listener, current: &current}
			downstream.Start()
			defer downstream.Close()
			for cycle := 0; cycle < 6; cycle++ {
				c := newNativeCancelCycle()
				current.Store(c)
				func() {
					defer func() { t.Logf("cycle %d %s:\n%s", cycle, mode, c) }()
					conn, err := net.DialTimeout("tcp", strings.TrimPrefix(downstream.URL, "http://"), time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					if err := conn.(*net.TCPConn).SetReadBuffer(1024); err != nil {
						t.Fatal(err)
					}
					if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
						t.Fatal(err)
					}
					if _, err := fmt.Fprintf(conn, "POST /api/cli/chat HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\nContent-Type: application/json\r\n\r\n%s", len(simpleBody), simpleBody); err != nil {
						t.Fatal(err)
					}
					reader := bufio.NewReader(conn)
					status, err := reader.ReadString('\n')
					if err != nil || !strings.Contains(status, "200 OK") {
						t.Fatalf("cycle %d: %q %v", cycle, status, err)
					}
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							t.Fatal(err)
						}
						if line == "\r\n" {
							break
						}
					}
					select {
					case <-c.started:
					case <-time.After(time.Second):
						t.Fatal("native generation not started")
					}
					cancelFrom := time.Now()
					if mode == "disconnect" {
						if err := conn.Close(); err != nil {
							t.Fatal(err)
						}
					}
					close(c.release)
					if mode == "nonreading" {
						select {
						case <-c.firstText:
						case <-time.After(time.Second):
							t.Fatal("no downstream text write; backpressure was not exercised")
						}
						var writeStart time.Time
						select {
						case writeStart = <-c.firstTCPWrite:
						case <-time.After(time.Second):
							t.Fatal("no downstream TCP payload write")
						}
						select {
						case timeout := <-c.timedOut:
							if timeout.written >= timeout.requested || !timeout.deadline.After(timeout.start) || timeout.end.Before(timeout.deadline) || timeout.deadline.Sub(timeout.start) > runtime.WriteTimeout {
								t.Fatalf("not a real bounded partial TCP write: %+v", timeout)
							}
							cancelFrom = timeout.end
						case <-time.After(time.Until(writeStart.Add(time.Second))):
							t.Fatal("nonreading TCP client did not exercise the 30ms write deadline")
						}
					}
					// One shared promptness budget, measured from disconnect or the
					// actual TCP timeout, not from upstream generation starting.
					timer := time.NewTimer(time.Until(cancelFrom.Add(time.Second)))
					defer timer.Stop()
					select {
					case at := <-c.upstreamCanceled:
						if at.After(cancelFrom.Add(time.Second)) {
							t.Fatal("native HTTP cancellation exceeded one second")
						}
					case <-timer.C:
						t.Fatal("downstream cancellation did not cancel native HTTP")
					}
					for _, done := range []<-chan struct{}{c.upstreamDone, c.downstreamDone} {
						select {
						case <-done:
						case <-timer.C:
							t.Fatal("canceled handler/admission slot did not release promptly")
						}
					}
					if time.Now().After(cancelFrom.Add(time.Second)) {
						t.Fatal("native cancellation and handler cleanup exceeded one second")
					}
					if requests.Load() != int32(cycle+1) {
						t.Fatal("retried generation", requests.Load())
					}
				}()
			}
			// More cycles than the admission limit plus a successful request demonstrate
			// that canceled generations release their slots, without a retry queue.
			current.Store(nil)
			resp, body := postStream(t, downstream.URL, simpleBody)
			if resp.StatusCode != 200 || strings.Count(body, "event: done") != 1 || requests.Load() != 7 {
				t.Fatal(resp.StatusCode, requests.Load(), body)
			}
		})
	}
}
