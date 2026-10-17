// Package testutil contains synthetic compatibility infrastructure only. It is
// never imported by the executable; its transport has no network fallback.
package testutil

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/application"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/httpserver"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
	"github.com/Jghh42/atuin-ai/tests/projection"
)

type Call struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}
type Turn struct {
	Text               string
	Calls              []Call
	Fail, Hold, Cancel bool
	UnknownUsage       bool
}
type Request struct {
	Body   json.RawMessage
	Header http.Header
}
type Native struct {
	t                            *testing.T
	mu                           sync.Mutex
	now                          time.Time
	turns                        []Turn
	upstream, requests           []Request
	sessions                     []string
	active                       map[int]bool
	downstream                   map[int][]byte
	heldIndex                    int
	barrierChecked               bool
	models, exchanges, refreshes int
	revoked                      bool
	token                        string
	release                      chan struct{}
	cancelled                    chan struct{}
	Runtime                      *application.Runtime
	HistoryIDFile                string // Task-owned synthetic fixture, set before starting CLI.
	URL                          string
}

func New(t *testing.T, turns []Turn, login bool) *Native {
	t.Helper()
	n := &Native{t: t, now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), turns: turns, active: map[int]bool{}, downstream: map[int][]byte{}, heldIndex: -1, release: make(chan struct{}, 1), cancelled: make(chan struct{}, 1)}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	r, err := application.NewWithOptions(filepath.Join(root, "credentials"), config.Default(), application.Options{Transport: n, Now: n.Clock, Wait: func(ctx context.Context, _ time.Duration) error { return ctx.Err() }})
	if err != nil {
		t.Fatal(err)
	}
	n.Runtime = r
	t.Cleanup(func() { _ = r.Close() })
	if login {
		n.Login()
	}
	server, err := httpserver.NewWithRuntime(config.Default(), nil, r.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, l) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(7 * time.Second):
			t.Error("native shutdown timeout")
		}
	})
	u, _ := url.Parse("http://" + l.Addr().String())
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.FlushInterval = -1
	proxy.ModifyResponse = func(r *http.Response) error {
		if r.Request.URL.Path == "/api/cli/chat" {
			n.mu.Lock()
			n.sessions = append(n.sessions, r.Header.Get("X-Atuin-AI-Session-Id"))
			n.mu.Unlock()
		}
		return nil
	}
	// Explicit loopback listener, including on hosts whose localhost resolves to IPv6.
	front := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__release" {
			if err := n.checkRelease(); err != nil {
				t.Error(err)
				http.Error(w, "premature executable frame", http.StatusConflict)
				return
			}
			select {
			case n.release <- struct{}{}:
			default:
			}
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == "/api/cli/chat" {
			b, e := io.ReadAll(io.LimitReader(r.Body, config.MaxRequestBodyBytes+1))
			if e != nil {
				t.Error(e)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(strings.NewReader(string(b)))
			n.mu.Lock()
			index := len(n.requests)
			n.requests = append(n.requests, Request{Body: b, Header: r.Header.Clone()})
			n.active[index] = true
			n.mu.Unlock()
			defer func() { n.mu.Lock(); delete(n.active, index); n.mu.Unlock() }()
			proxy.ServeHTTP(&recordingWriter{ResponseWriter: w, n: n, index: index}, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	_ = front.Listener.Close()
	front.Listener, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	front.Start()
	t.Cleanup(front.Close)
	n.URL = front.URL
	return n
}
func (n *Native) Clock() time.Time { n.mu.Lock(); defer n.mu.Unlock(); return n.now }
func (n *Native) Expire(revoked bool) {
	n.mu.Lock()
	n.now = n.now.Add(time.Hour)
	n.revoked = revoked
	n.mu.Unlock()
}
func (n *Native) Login() {
	n.t.Helper()
	if err := n.Runtime.Manager.Login(context.Background(), func(p credentials.DevicePrompt) error {
		if p.UserCode != "FAKE-ONLY" {
			return fmt.Errorf("unexpected synthetic prompt")
		}
		return nil
	}); err != nil {
		n.t.Fatal(err)
	}
}
func (n *Native) Captures() ([]Request, []Request, int, int, int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]Request(nil), n.requests...), append([]Request(nil), n.upstream...), n.models, n.exchanges, n.refreshes
}
func (n *Native) Sessions() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.sessions...)
}
func (n *Native) AwaitCancel() {
	n.t.Helper()
	select {
	case <-n.cancelled:
	case <-time.After(3 * time.Second):
		n.t.Fatal("fake upstream not cancelled")
	}
}
func reply(status int, s string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(s))}
}
func frame(kind string, v any) string {
	b, _ := json.Marshal(v)
	return "event: " + kind + "\ndata: " + string(b) + "\n\n"
}
func textFrame(text string) string {
	return frame("response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "message", "id": "message-1", "role": "assistant", "content": []any{}}}) + frame("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": "message-1", "output_index": 0, "content_index": 0, "delta": text})
}
func output(turn Turn) []any {
	out := []any{}
	if turn.Text != "" {
		out = append(out, map[string]any{"type": "message", "id": "message-1", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": turn.Text}}})
	}
	for i, c := range turn.Calls {
		out = append(out, map[string]any{"type": "function_call", "id": fmt.Sprintf("item-%d", i), "call_id": c.ID, "name": c.Name, "arguments": string(c.Input)})
	}
	return out
}
func completed(turn Turn) string {
	response := map[string]any{"id": "synthetic-response", "status": "completed", "output": output(turn)}
	if !turn.UnknownUsage {
		response["usage"] = map[string]int{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}
	}
	return frame("response.completed", map[string]any{"type": "response.completed", "response": response})
}
func (n *Native) RoundTrip(r *http.Request) (*http.Response, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	switch r.URL.String() {
	case "https://auth.openai.com/api/accounts/deviceauth/usercode":
		return reply(200, `{"device_auth_id":"fake-device","user_code":"FAKE-ONLY","interval":1}`), nil
	case "https://auth.openai.com/api/accounts/deviceauth/token":
		return reply(200, `{"authorization_code":"fake-code","code_verifier":"`+strings.Repeat("v", 43)+`"}`), nil
	case "https://auth.openai.com/oauth/token":
		if err := r.ParseForm(); err != nil {
			n.t.Error(err)
		}
		if r.Form.Get("grant_type") == "refresh_token" {
			n.refreshes++
			if n.revoked {
				return reply(400, `{"error":"invalid_grant"}`), nil
			}
		} else {
			n.exchanges++
		}
		b, _ := json.Marshal(map[string]any{"exp": n.now.Add(time.Hour).Unix(), "https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "synthetic-account"}})
		token := "e30." + base64.RawURLEncoding.EncodeToString(b) + ".c2ln"
		n.token = token
		b, _ = json.Marshal(map[string]any{"access_token": token, "refresh_token": fmt.Sprintf("fake-refresh-%d-%d", n.exchanges, n.refreshes), "expires_in": 3600})
		return reply(200, string(b)), nil
	case codex.ModelsURL:
		n.models++
		if r.Header.Get("ChatGPT-Account-Id") != "synthetic-account" || r.Header.Get("Authorization") != "Bearer "+n.token {
			n.t.Error("account binding lost")
		}
		return reply(200, `{"models":[{"slug":"gpt-6.1-sol","display_name":"Sol","visibility":"list","priority":0}]}`), nil
	case codex.ResponsesURL:
		b, e := io.ReadAll(r.Body)
		if e != nil {
			return nil, e
		}
		n.upstream = append(n.upstream, Request{b, r.Header.Clone()})
		if len(n.active) != 1 {
			n.t.Error("semantic fake requires one active serialized Atuin request")
			return reply(500, `{}`), nil
		}
		requestIndex := -1
		for index := range n.active {
			requestIndex = index
		}
		if e := projection.Validate("responses", n.requests[requestIndex].Body, b); e != nil {
			n.t.Errorf("native outbound semantics: %v", e)
			return reply(500, `{}`), nil
		}
		if r.Header.Get("ChatGPT-Account-Id") != "synthetic-account" || r.Header.Get("Authorization") != "Bearer "+n.token {
			n.t.Error("credential/account binding lost")
		}
		i := len(n.upstream) - 1
		if i >= len(n.turns) {
			n.t.Errorf("unexpected generation %d", i+1)
			return reply(500, `{}`), nil
		}
		turn := n.turns[i]
		if n.HistoryIDFile != "" {
			b, err := os.ReadFile(n.HistoryIDFile)
			if err != nil {
				return nil, err
			}
			turn.Calls = append([]Call(nil), turn.Calls...)
			for j := range turn.Calls {
				turn.Calls[j].Input = json.RawMessage(strings.ReplaceAll(string(turn.Calls[j].Input), "SYNTHETIC_HISTORY_ID", strings.TrimSpace(string(b))))
			}
		}
		body := ""
		if turn.Text != "" {
			body = textFrame(turn.Text)
		}
		if turn.Fail || turn.Hold || turn.Cancel {
			announced := ""
			for j, c := range turn.Calls {
				index := j
				if turn.Text != "" {
					index++
				}
				item := map[string]any{"type": "function_call", "id": fmt.Sprintf("item-%d", j), "call_id": c.ID, "name": c.Name, "arguments": string(c.Input)}
				announced += frame("response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": index, "item": item})
				announced += frame("response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": index, "item": item})
			}
			// Parsing the visible witness now necessarily follows parsing the
			// complete callable snapshot, not merely receiving a TCP buffer.
			body = announced + body
		}
		if turn.Fail {
			return reply(200, body+frame("response.failed", map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed", "error": map[string]string{"message": "synthetic-private-detail"}}})), nil
		}
		if turn.Hold || turn.Cancel {
			if turn.Hold {
				n.heldIndex = requestIndex
			}
			reader, writer := io.Pipe()
			go func() {
				defer writer.Close()
				if _, err := io.WriteString(writer, body); err != nil {
					return
				}
				select {
				case <-r.Context().Done():
					select {
					case n.cancelled <- struct{}{}:
					default:
					}
				case <-n.release:
					_, _ = io.WriteString(writer, completed(turn))
				}
			}()
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: reader}, nil
		}
		return reply(200, body+completed(turn)), nil
	default:
		n.t.Errorf("unexpected native endpoint: %s", r.URL.Redacted())
		return nil, fmt.Errorf("fake transport denies all other destinations")
	}
}

// Post sends only a synthetic request and never logs response bodies or tokens.
func Post(t *testing.T, base string, body []byte) (int, string) {
	t.Helper()
	status, text, _ := PostWithSession(t, base, body)
	return status, text
}
func PostWithSession(t *testing.T, base string, body []byte) (int, string, string) {
	t.Helper()
	c := http.Client{Timeout: 8 * time.Second}
	r, e := c.Post(base+"/api/cli/chat", "application/json", strings.NewReader(string(body)))
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if e != nil {
		t.Fatal(e)
	}
	return r.StatusCode, string(b), r.Header.Get("X-Atuin-AI-Session-Id")
}
