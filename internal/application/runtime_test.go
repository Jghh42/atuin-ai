package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/httpserver"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

type fakeNative struct {
	t                                              *testing.T
	mu                                             sync.Mutex
	now                                            time.Time
	token                                          string
	refresh                                        string
	exchanges, rotations, discoveries, generations int
	// Optional per-fetch handshake, installed only with no active fetch.
	fetchEntered chan struct{}
	fetchRelease chan struct{}
}

func (f *fakeNative) clock() time.Time        { f.mu.Lock(); defer f.mu.Unlock(); return f.now }
func (f *fakeNative) advance(d time.Duration) { f.mu.Lock(); f.now = f.now.Add(d); f.mu.Unlock() }
func syntheticJWT(now time.Time) string {
	b, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "test-account-private"}, "exp": now.Add(time.Hour).Unix()})
	return "e30." + base64.RawURLEncoding.EncodeToString(b) + ".c2ln"
}
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func (f *fakeNative) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Scheme != "https" {
		f.t.Error("native destination not HTTPS")
	}
	switch r.URL.String() {
	case "https://auth.openai.com/api/accounts/deviceauth/usercode":
		return response(200, `{"device_auth_id":"fake-device","user_code":"FAKE-CODE","interval":1}`), nil
	case "https://auth.openai.com/api/accounts/deviceauth/token":
		return response(200, `{"authorization_code":"fake-authcode","code_verifier":"`+strings.Repeat("v", 43)+`"}`), nil
	case "https://auth.openai.com/oauth/token":
		if e := r.ParseForm(); e != nil {
			f.t.Error("bad form")
		}
		if r.Form.Get("client_id") != credentials.ClientID {
			f.t.Error("wrong registration")
		}
		if r.Form.Get("grant_type") == "refresh_token" {
			f.rotations++
			if r.Form.Get("refresh_token") != f.refresh {
				f.t.Error("old refresh token reused")
			}
		} else {
			f.exchanges++
		}
		f.token = syntheticJWT(f.now)
		f.refresh = fmt.Sprintf("fake-refresh-%d-%d", f.exchanges, f.rotations)
		b, _ := json.Marshal(map[string]any{"access_token": f.token, "refresh_token": f.refresh, "expires_in": 3600})
		return response(200, string(b)), nil
	case codex.ModelsURL:
		f.checkCredential(r)
		f.discoveries++
		entered, release := f.fetchEntered, f.fetchRelease
		f.fetchEntered = nil
		f.fetchRelease = nil
		if entered != nil {
			close(entered)
			f.mu.Unlock()
			<-release
			f.mu.Lock()
		}
		return response(200, `{"models":[{"slug":"gpt-6.1-sol","display_name":"Sol","visibility":"list","priority":0}]}`), nil
	case codex.ResponsesURL:
		f.checkCredential(r)
		f.generations++
		body, _ := io.ReadAll(r.Body)
		var wire struct {
			Model string           `json:"model"`
			Input []map[string]any `json:"input"`
		}
		if json.Unmarshal(body, &wire) != nil || wire.Model != "gpt-6.1-sol" {
			f.t.Error("wrong generation projection")
		}
		var output []any
		if f.generations == 2 {
			output = []any{map[string]any{"type": "function_call", "id": "item-read", "call_id": "call-read", "name": "read_file", "arguments": `{"file_path":"fixture.txt","offset":0,"limit":10}`}}
		} else {
			if f.generations == 3 {
				found := false
				for _, item := range wire.Input {
					if item["type"] == "function_call_output" {
						found = item["call_id"] == "call-read" && item["output"] == "synthetic file contents"
					}
				}
				if !found {
					f.t.Error("tool continuation not replayed")
				}
			}
			output = []any{map[string]any{"type": "message", "id": "msg", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "synthetic answer"}}}}
		}
		event, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp", "status": "completed", "output": output}})
		return response(200, "event: response.completed\ndata: "+string(event)+"\n\n"), nil
	}
	f.t.Error("unreviewed native endpoint")
	return response(500, `{}`), nil
}
func (f *fakeNative) checkCredential(r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+f.token || r.Header.Get("ChatGPT-Account-Id") != "test-account-private" {
		f.t.Error("token/account pair not bound")
	}
}
func privatePath(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	return filepath.Join(root, "credentials")
}
func runtime(t *testing.T, path string, f *fakeNative) *Runtime {
	t.Helper()
	r, e := NewWithOptions(path, config.Default(), Options{Transport: f, Now: f.clock})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	return r
}
func startServer(t *testing.T, r *Runtime) (string, func()) {
	t.Helper()
	server, e := httpserver.NewWithRuntime(config.Default(), nil, r.HTTP)
	if e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case e := <-done:
				if e != nil {
					t.Error(e)
				}
			case <-time.After(7 * time.Second):
				t.Error("server shutdown timed out")
			}
		})
	}
	t.Cleanup(stop)
	return "http://" + listener.Addr().String(), stop
}
func api(t *testing.T, base, route, body string) (int, string) {
	t.Helper()
	method := "GET"
	if body != "" {
		method = "POST"
	}
	req, _ := http.NewRequest(method, base+route, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 5 * time.Second}
	res, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	b, e := io.ReadAll(res.Body)
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"test-account-private", "fake-refresh", "Bearer ", "fake-authcode", "FAKE-CODE"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("API secret disclosure")
		}
	}
	return res.StatusCode, string(b)
}
func TestComposedNativeLoginDiscoveryToolRefreshRestartLogout(t *testing.T) {
	f := &fakeNative{t: t, now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	path := privatePath(t)
	r := runtime(t, path, f)
	base, stop := startServer(t, r)
	text := `{"messages":[{"role":"user","content":"hello"}]}`
	for _, route := range []string{"/api/cli/models", "/api/cli/chat"} {
		body := ""
		if strings.HasSuffix(route, "chat") {
			body = text
		}
		if status, _ := api(t, base, route, body); status != 503 {
			t.Fatal("missing credentials not 503")
		}
	}
	if e := r.Manager.Login(context.Background(), func(p credentials.DevicePrompt) error {
		if p.UserCode != "FAKE-CODE" {
			t.Error("wrong private prompt")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if status, b := api(t, base, "/api/cli/models", ""); status != 200 || !strings.Contains(b, `"default":"gpt-6.1-sol"`) {
		t.Fatal("login discovery failed")
	}
	if status, b := api(t, base, "/api/cli/chat", text); status != 200 || !strings.Contains(b, "event: text") || !strings.Contains(b, "event: done") {
		t.Fatal("native text pipeline failed")
	}
	tool := `{"messages":[{"role":"user","content":"read fixture"}],"config":{"capabilities":["client_v1_read_file"]}}`
	if status, b := api(t, base, "/api/cli/chat", tool); status != 200 || !strings.Contains(b, `"id":"call-read"`) || !strings.Contains(b, "waiting_for_tools") || !strings.Contains(b, "event: done") {
		t.Fatal("native tool pipeline failed")
	}
	continuation := `{"messages":[{"role":"user","content":"read fixture"},{"role":"assistant","content":[{"type":"tool_use","id":"call-read","name":"read_file","input":{"file_path":"fixture.txt","offset":0,"limit":10}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-read","content":"synthetic file contents"}]}],"config":{"capabilities":["client_v1_read_file"]}}`
	if status, b := api(t, base, "/api/cli/chat", continuation); status != 200 || !strings.Contains(b, "event: done") || strings.Contains(b, "event: error") {
		t.Fatal("native continuation failed")
	}
	epoch := r.Manager.Status().AuthEpoch
	f.advance(time.Hour - credentials.AccessSkew)
	if _, e := r.Manager.Current(context.Background()); e != nil {
		t.Fatal(e)
	}
	if r.Manager.Status().AuthEpoch != epoch {
		t.Fatal("rotation changed auth epoch")
	}
	stop()
	r.Close()
	r2 := runtime(t, path, f)
	base2, stop2 := startServer(t, r2)
	if status, b := api(t, base2, "/api/cli/chat", text); status != 200 || !strings.Contains(b, "event: done") {
		t.Fatal("restart inference failed")
	}
	if e := r2.Manager.Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	if status, _ := api(t, base2, "/api/cli/models", ""); status != 503 {
		t.Fatal("catalog survived logout")
	}
	if status, _ := api(t, base2, "/api/cli/chat", text); status != 503 {
		t.Fatal("chat survived logout")
	}
	stop2()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.exchanges != 1 || f.rotations != 1 || f.generations != 4 || f.discoveries != 2 {
		t.Fatal("unexpected native lifecycle call counts", f.exchanges, f.rotations, f.generations, f.discoveries)
	}
}
func TestSameAccountLoginAndLogoutIsolateInflightCatalog(t *testing.T) {
	f := &fakeNative{t: t, now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	r := runtime(t, privatePath(t), f)
	login := func() {
		if e := r.Manager.Login(context.Background(), func(credentials.DevicePrompt) error { return nil }); e != nil {
			t.Fatal(e)
		}
	}
	login()
	for _, action := range []string{"same-account-login", "logout"} {
		t.Run(action, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			f.mu.Lock()
			f.fetchEntered = entered
			f.fetchRelease = release
			f.mu.Unlock()
			done := make(chan error, 1)
			go func() { _, e := r.Catalog.Get(context.Background()); done <- e }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("fetch not entered")
			}
			if action == "same-account-login" {
				login()
			} else {
				if e := r.Manager.Logout(context.Background()); e != nil {
					t.Fatal(e)
				}
			}
			close(release)
			select {
			case e := <-done:
				if e == nil {
					t.Fatal("obsolete discovery survived lifecycle")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("old catalog waiter stranded")
			}
			snapshot, e := r.Catalog.Get(context.Background())
			if action == "same-account-login" {
				if e != nil || snapshot.Identity().Epoch != 2 {
					t.Fatal("new epoch not bound")
				}
				f.advance(catalog.FreshFor)
			} else if e == nil {
				t.Fatal("logged-out catalog repopulated")
			}
		})
	}
}
