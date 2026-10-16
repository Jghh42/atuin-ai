package httpserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

// Couple both real TCP legs: Atuin disconnect/write timeout must cancel the
// native Codex HTTP request, not merely a stand-in Provider implementation.
func TestNativeCancellationChainReleasesAdmission(t *testing.T) {
	for _, mode := range []string{"disconnect", "nonreading"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			var healthy atomic.Bool
			started, closed := make(chan struct{}, 1), make(chan struct{}, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				if healthy.Load() {
					fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp\",\"status\":\"completed\",\"output\":[]}}\n\n")
					return
				}
				defer func() { closed <- struct{}{} }()
				controller := http.NewResponseController(w)
				// Bound the fake upstream too, so a failed test cannot retain its handler.
				_ = controller.SetWriteDeadline(time.Now().Add(2 * time.Second))
				fmt.Fprint(w, "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"msg\",\"type\":\"message\",\"content\":[]}}\n\n")
				_ = controller.Flush()
				started <- struct{}{}
				text := "visible"
				if mode == "nonreading" {
					text = strings.Repeat("x", 64<<10)
				}
				b, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "item_id": "msg", "delta": text})
				for {
					if _, err := fmt.Fprintf(w, "event: response.output_text.delta\ndata: %s\n\n", b); err != nil {
						break
					}
					if err := controller.Flush(); err != nil {
						break
					}
					if mode == "disconnect" {
						break
					}
					select {
					case <-r.Context().Done():
						return
					default:
					}
				}
				// Report closure only after the HTTP request context actually canceled.
				select {
				case <-r.Context().Done():
				case <-time.After(2 * time.Second):
					t.Error("native request did not close promptly")
				}
			}))
			defer upstream.Close()
			runtime := testRuntime(t, nil)
			runtime.Provider = codex.RequestClient{Credentials: runtime.Credentials, URL: upstream.URL}
			runtime.WriteTimeout = 30 * time.Millisecond
			downstream := runtimeServer(t, runtime)
			for cycle := 0; cycle < 6; cycle++ {
				conn, err := net.Dial("tcp", strings.TrimPrefix(downstream.URL, "http://"))
				if err != nil {
					t.Fatal(err)
				}
				if tcp, ok := conn.(*net.TCPConn); ok {
					_ = tcp.SetReadBuffer(1024)
				}
				_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				fmt.Fprintf(conn, "POST /api/cli/chat HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\nContent-Type: application/json\r\n\r\n%s", len(simpleBody), simpleBody)
				reader := bufio.NewReader(conn)
				status, err := reader.ReadString('\n')
				if err != nil || !strings.Contains(status, "200 OK") {
					_ = conn.Close()
					t.Fatalf("cycle %d: %q %v", cycle, status, err)
				}
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						_ = conn.Close()
						t.Fatal(err)
					}
					if line == "\r\n" {
						break
					}
				}
				select {
				case <-started:
				case <-time.After(time.Second):
					_ = conn.Close()
					t.Fatal("native generation not started")
				}
				if mode == "disconnect" {
					_ = conn.Close()
				}
				select {
				case <-closed:
				case <-time.After(time.Second):
					_ = conn.Close()
					t.Fatalf("cycle %d: downstream %s did not cancel native HTTP", cycle, mode)
				}
				_ = conn.Close()
				if requests.Load() != int32(cycle+1) {
					t.Fatal("retried generation", requests.Load())
				}
			}
			// More cycles than the admission limit plus a successful request demonstrate
			// that canceled generations release their slots, without a retry queue.
			healthy.Store(true)
			resp, body := postStream(t, downstream.URL, simpleBody)
			if resp.StatusCode != 200 || strings.Count(body, "event: done") != 1 || requests.Load() != 7 {
				t.Fatal(resp.StatusCode, requests.Load(), body)
			}
		})
	}
}
