package httpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

type streamProvider func(context.Context, provider.Request, func(provider.Event) error) (provider.Usage, error)

func (f streamProvider) Stream(c context.Context, r provider.Request, s func(provider.Event) error) (provider.Usage, error) {
	return f(c, r, s)
}

type fixedCatalog struct{ snapshot catalog.Snapshot }

func (f fixedCatalog) Get(context.Context) (catalog.Snapshot, error) { return f.snapshot, nil }
func testRuntime(t *testing.T, p provider.Provider) Runtime {
	t.Helper()
	snapshot, err := catalog.Build([]catalog.Entry{{ID: "gpt-6.1-sol", Name: "Sol", Visibility: "list"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	return Runtime{Catalog: fixedCatalog{snapshot}, Provider: p, Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
		return credentials.Credential{AccessToken: "private", Identity: credentials.Identity{Provider: "codex", Account: "private"}}, nil
	})}
}
func runtimeServer(t *testing.T, r Runtime) *httptest.Server {
	t.Helper()
	s, err := NewWithRuntime(config.Default(), nil, r)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.http.Handler)
	t.Cleanup(ts.Close)
	return ts
}

const session = "018f7f33-0ad3-7aef-bf0a-d204a18a1234"
const simpleBody = `{"messages":[{"role":"user","content":"hi"}],"session_id":"` + session + `"}`

func postStream(t *testing.T, base, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(base+"/api/cli/chat", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

func TestExactTCPFramesAndHeaders(t *testing.T) {
	var n atomic.Int32
	ts := runtimeServer(t, testRuntime(t, streamProvider(func(_ context.Context, r provider.Request, s func(provider.Event) error) (provider.Usage, error) {
		n.Add(1)
		if r.Conversation.Config.ProviderModel != "gpt-6.1-sol" {
			t.Error("wrong resolution")
		}
		err := s(provider.Event{Kind: provider.TextDelta, Text: "hello\n\"world\""})
		return provider.Usage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10, CachedTokens: 2, InputKnown: true, OutputKnown: true, TotalKnown: true, CachedKnown: true}, err
	})))
	resp, body := postStream(t, ts.URL, simpleBody)
	for key, want := range map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-cache", "X-Accel-Buffering": "no", "X-Atuin-AI-Session-Id": session} {
		if resp.Header.Get(key) != want {
			t.Fatal(key, resp.Header)
		}
	}
	want := "event: status\ndata: {\"state\":\"processing\"}\n\nevent: status\ndata: {\"state\":\"thinking\"}\n\nevent: text\ndata: {\"content\":\"hello\\n\\\"world\\\"\"}\n\nevent: done\ndata: {\"session_id\":\"" + session + "\",\"usage\":{\"cache_creation_tokens\":null,\"cached_tokens\":2,\"input_cost\":null,\"input_tokens\":7,\"output_cost\":null,\"output_tokens\":3,\"provider_cost\":null,\"total_cost\":null,\"total_tokens\":10}}\n\n"
	if resp.StatusCode != 200 || body != want || n.Load() != 1 {
		t.Fatalf("%d %d\n%s", resp.StatusCode, n.Load(), body)
	}
	// Missing sessions are generated before headers and agree with done.
	resp, body = postStream(t, ts.URL, `{"messages":[{"role":"user","content":"hi"}]}`)
	id := resp.Header.Get("X-Atuin-AI-Session-Id")
	if len(id) != 36 || !strings.Contains(body, `"session_id":"`+id+`"`) {
		t.Fatal(id, body)
	}
}

func TestPreStreamFailuresNeverGenerate(t *testing.T) {
	var n atomic.Int32
	p := streamProvider(func(context.Context, provider.Request, func(provider.Event) error) (provider.Usage, error) {
		n.Add(1)
		return provider.Usage{}, nil
	})
	for _, which := range []string{"decode", "auth", "catalog", "credential timeout"} {
		t.Run(which, func(t *testing.T) {
			r := testRuntime(t, p)
			body := simpleBody
			switch which {
			case "decode":
				body = `{"messages":[]}`
			case "auth":
				r.Credentials = credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
					return credentials.Credential{}, errors.New("secret")
				})
			case "catalog":
				r.Catalog = &changingCatalog{}
			case "credential timeout":
				r.PreflightTimeout = 10 * time.Millisecond
				r.Credentials = credentials.SourceFunc(func(ctx context.Context) (credentials.Credential, error) {
					<-ctx.Done()
					return credentials.Credential{}, ctx.Err()
				})
			}
			ts := runtimeServer(t, r)
			resp, b := postStream(t, ts.URL, body)
			if resp.StatusCode < 400 || resp.Header.Get("X-Atuin-AI-Session-Id") != "" || strings.Contains(b, "event:") || strings.Contains(b, "secret") {
				t.Fatal(resp.StatusCode, b)
			}
		})
	}
	if n.Load() != 0 {
		t.Fatal(n.Load())
	}
}

func TestNativeResponsesToAtuinToolsAndContinuation(t *testing.T) {
	inputs := map[string]string{
		"suggest_command": `{"command":"echo ok","description":"x","confidence":"high","danger":"low"}`,
		"read_file":       `{"file_path":"x","precision":9007199254740993,"decimal":1.2300}`, "write_file": `{"file_path":"x","content":"x"}`, "edit_file": `{"file_path":"x","old_string":"a","new_string":"b"}`,
		"atuin_history": `{"query":"x","filter_modes":["global"]}`, "atuin_output": `{"history_id":"x"}`, "execute_shell_command": `{"command":"echo ok","shell":"bash","description":"x"}`, "load_skill": `{"name":"x"}`,
	}
	for _, name := range []string{"text", "suggest_command", "read_file", "write_file", "edit_file", "atuin_history", "atuin_output", "execute_shell_command", "load_skill", "parallel", "failure", "mixed", "schema", "hosted", "continuation"} {
		t.Run(name, func(t *testing.T) {
			var n atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n.Add(1)
				var outbound map[string]any
				_ = json.NewDecoder(r.Body).Decode(&outbound)
				if name == "continuation" {
					if _, present := outbound["previous_response_id"]; present {
						t.Error("provider state used instead of full history")
					}
					input, ok := outbound["input"].([]any)
					expected := []any{
						map[string]any{"role": "user", "content": "original question"},
						map[string]any{"role": "assistant", "content": "checking"},
						map[string]any{"type": "function_call", "call_id": "old-a", "name": "read_file", "arguments": inputs["read_file"]},
						map[string]any{"type": "function_call", "call_id": "old-b", "name": "write_file", "arguments": inputs["write_file"]},
						map[string]any{"type": "function_call_output", "call_id": "old-b", "output": "Tool execution failed: permission denied"},
						map[string]any{"type": "function_call_output", "call_id": "old-a", "output": "original result\n9007199254740993"},
						map[string]any{"role": "user", "content": "now summarize"},
					}
					if !ok || len(input) != len(expected)+1 {
						t.Errorf("full history shape: %#v", outbound)
						return
					}
					if !reflect.DeepEqual(input[1:], expected) {
						t.Errorf("history/call correlation mismatch:\ngot %#v\nwant %#v", input[1:], expected)
					}
					contextItem, ok := input[0].(map[string]any)
					if !ok || contextItem["role"] != "user" || contextItem["content"] == "" {
						t.Errorf("missing separate turn context: %#v", input[0])
					}
				}
				outputs := []any{}
				add := func(tool, id string) {
					outputs = append(outputs, map[string]any{"type": "function_call", "id": "item_" + id, "call_id": id, "name": tool, "arguments": inputs[tool]})
				}
				switch name {
				case "text", "continuation":
					outputs = append(outputs, map[string]any{"type": "message", "id": "msg", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "hello\nworld"}}})
				case "parallel":
					add("read_file", "a")
					add("write_file", "b")
				case "mixed":
					add("suggest_command", "a")
					add("read_file", "b")
				case "schema":
					outputs = append(outputs, map[string]any{"type": "function_call", "id": "i", "call_id": "a", "name": "read_file", "arguments": "{}"})
				case "hosted":
					outputs = append(outputs, map[string]any{"type": "web_search_call", "id": "i"})
				case "failure":
					fmt.Fprint(w, "event: error\ndata: {\"type\":\"error\",\"message\":\"private provider content\"}\n\n")
					return
				default:
					add(name, "a")
				}
				b, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp", "status": "completed", "output": outputs}})
				fmt.Fprintf(w, "event: response.completed\ndata: %s\n\n", b)
			}))
			defer upstream.Close()
			r := testRuntime(t, nil)
			r.Provider = codex.RequestClient{Credentials: r.Credentials, URL: upstream.URL}
			ts := runtimeServer(t, r)
			request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}, "config": map[string]any{"capabilities": []string{"client_v1_read_file", "client_v1_write_file", "client_v1_edit_file", "client_v1_atuin_history", "client_v1_atuin_output", "client_v1_execute_shell_command", "client_v1_load_skill"}}}
			if name == "continuation" {
				request["messages"] = []any{
					map[string]any{"role": "user", "content": "original question"},
					map[string]any{"role": "assistant", "content": []any{
						map[string]any{"type": "text", "text": "checking"},
						map[string]any{"type": "tool_use", "id": "old-a", "name": "read_file", "input": json.RawMessage(inputs["read_file"])},
						map[string]any{"type": "tool_use", "id": "old-b", "name": "write_file", "input": json.RawMessage(inputs["write_file"])},
					}},
					map[string]any{"role": "user", "content": []any{
						map[string]any{"type": "tool_result", "tool_use_id": "old-b", "content": "permission denied", "is_error": true},
						map[string]any{"type": "tool_result", "tool_use_id": "old-a", "content": "original result\n9007199254740993"},
						map[string]any{"type": "text", "text": "now summarize"},
					}},
				}
			}
			raw, _ := json.Marshal(request)
			resp, body := postStream(t, ts.URL, string(raw))
			if resp.StatusCode != 200 || n.Load() != 1 {
				t.Fatal(resp.StatusCode, n.Load(), body)
			}
			invalid := name == "mixed" || name == "failure" || name == "schema" || name == "hosted"
			if invalid {
				if strings.Count(body, "event: error") != 1 || strings.Contains(body, "event: done") || strings.Contains(body, "event: tool_call") || strings.Contains(body, "private") {
					t.Fatal(body)
				}
				return
			}
			if strings.Count(body, "event: done") != 1 || strings.Contains(body, "event: error") {
				t.Fatal(body)
			}
			toolCount := 1
			if name == "text" || name == "continuation" {
				toolCount = 0
			}
			if name == "parallel" {
				toolCount = 2
			}
			if strings.Count(body, "event: tool_call") != toolCount {
				t.Fatal(body)
			}
			if strings.Contains(body, "waiting_for_tools") != (toolCount > 0 && name != "suggest_command") {
				t.Fatal(body)
			}
			var gotCalls []tools.Call
			for _, frame := range strings.Split(body, "\n\n") {
				if !strings.HasPrefix(frame, "event: tool_call\ndata: ") {
					continue
				}
				var call tools.Call
				if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "event: tool_call\ndata: ")), &call); err != nil {
					t.Fatal(err)
				}
				gotCalls = append(gotCalls, call)
			}
			var expectedCalls []tools.Call
			if toolCount == 1 {
				expectedCalls = []tools.Call{{ID: "a", Name: name, Input: json.RawMessage(inputs[name])}}
			}
			if toolCount == 2 {
				expectedCalls = []tools.Call{{ID: "a", Name: "read_file", Input: json.RawMessage(inputs["read_file"])}, {ID: "b", Name: "write_file", Input: json.RawMessage(inputs["write_file"])}}
			}
			if !reflect.DeepEqual(gotCalls, expectedCalls) {
				t.Fatalf("exact call_id/name/arguments/order mismatch:\ngot %#v\nwant %#v", gotCalls, expectedCalls)
			}
		})
	}
}

func TestDisconnectCancelsProvider(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	ts := runtimeServer(t, testRuntime(t, streamProvider(func(ctx context.Context, _ provider.Request, _ func(provider.Event) error) (provider.Usage, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return provider.Usage{}, ctx.Err()
	})))
	resp, err := http.Post(ts.URL+"/api/cli/chat", "application/json", strings.NewReader(simpleBody))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	_ = resp.Body.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("provider not canceled")
	}
}

func TestSlowTCPClientWriteDeadline(t *testing.T) {
	stopped := make(chan struct{})
	r := testRuntime(t, streamProvider(func(ctx context.Context, _ provider.Request, s func(provider.Event) error) (provider.Usage, error) {
		defer close(stopped)
		for i := 0; i < 10000; i++ {
			if err := s(provider.Event{Kind: provider.TextDelta, Text: strings.Repeat("x", 64<<10)}); err != nil {
				if ctx.Err() == nil {
					t.Error("sink failure did not cancel")
				}
				return provider.Usage{}, err
			}
		}
		return provider.Usage{}, nil
	}))
	r.WriteTimeout = 30 * time.Millisecond
	ts := runtimeServer(t, r)
	conn, err := net.Dial("tcp", strings.TrimPrefix(ts.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(1024)
	}
	fmt.Fprintf(conn, "POST /api/cli/chat HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\nContent-Type: application/json\r\n\r\n%s", len(simpleBody), simpleBody)
	// Read only headers, then stop consuming the large streaming response.
	reader := bufio.NewReader(conn)
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
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("slow writer held upstream")
	}
}
