package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/prompt"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

func discoveryCredential() credentials.Credential { return credential() }

var projectionTime = time.Date(2026, 10, 16, 23, 0, 0, 0, time.UTC)

func decodedRequest(t *testing.T, messages string) provider.Request {
	t.Helper()
	d := atuin.Decoder{Catalog: atuin.ModelCatalog{Default: "test-model", Models: map[string]string{"test-model": "test-model"}}}
	r, err := d.Decode([]byte(`{"messages":` + messages + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return provider.Request{Conversation: r}
}

func textMessage(role, text string) map[string]any {
	return map[string]any{"role": role, "content": text}
}
func callItem(id, name, args string) map[string]any {
	return map[string]any{"type": "function_call", "call_id": id, "name": name, "arguments": args}
}
func outputItemExpected(id, output string) map[string]any {
	return map[string]any{"type": "function_call_output", "call_id": id, "output": output}
}

// Original synthetic fixtures, not captured account/model transcripts. These
// exercise the probe/Pi wire contract, including IDs that must not be split or
// rewritten to output-item IDs, and JSON numeric spellings that float64 loses.
func TestExactOutboundRequests(t *testing.T) {
	cases := []struct {
		name, messages string
		input          []any
	}{
		{"text", `[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"},{"role":"user","content":"again"}]`, []any{textMessage("user", "hello"), textMessage("assistant", "hi"), textMessage("user", "again")}},
		{"suggestion before next user turn", `[{"role":"user","content":"list files"},{"role":"assistant","content":[{"type":"tool_use","id":"suggest-1","name":"suggest_command","input":{"command":"ls"}}]}]`, []any{textMessage("user", "list files"), textMessage("assistant", `Terminal command suggestion (suggest_command; suggestion only, not evidence of execution). Original arguments JSON: {"command":"ls"}`)}},
		{"suggestion after next user turn", `[{"role":"user","content":"list files"},{"role":"assistant","content":[{"type":"tool_use","id":"suggest-1","name":"suggest_command","input":{ "command":"ls", "unknown":[9007199254740993,1.2300e+04,-0] }}]},{"role":"user","content":"why?"},{"role":"assistant","content":[{"type":"tool_use","id":"read-1","name":"read_file","input":{"file_path":"a"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"read-1","content":"contents"}]}]`, []any{textMessage("user", "list files"), textMessage("assistant", `Terminal command suggestion (suggest_command; suggestion only, not evidence of execution). Original arguments JSON: { "command":"ls", "unknown":[9007199254740993,1.2300e+04,-0] }`), textMessage("user", "why?"), callItem("read-1", "read_file", `{"file_path":"a"}`), outputItemExpected("read-1", "contents")}},
		{"single", `[{"role":"assistant","content":[{"type":"tool_use","id":"call-not-fc-item","name":"read_file","input":{"file_path":"a","unknown":[9007199254740993,1.2300e+04,-0,null,true]}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-not-fc-item","content":"contents"}]}]`, []any{callItem("call-not-fc-item", "read_file", `{"file_path":"a","unknown":[9007199254740993,1.2300e+04,-0,null,true]}`), outputItemExpected("call-not-fc-item", "contents")}},
		{"parallel failed", `[{"role":"assistant","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"a","name":"read_file","input":{"file_path":"a"}},{"type":"tool_use","id":"b","name":"read_file","input":{"file_path":"b"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":"permission denied","is_error":true},{"type":"text","text":"keep going"},{"type":"tool_result","tool_use_id":"a","content":"ok"}]}]`, []any{textMessage("assistant", "checking"), callItem("a", "read_file", `{"file_path":"a"}`), callItem("b", "read_file", `{"file_path":"b"}`), outputItemExpected("b", "Tool execution failed: permission denied"), textMessage("user", "keep going"), outputItemExpected("a", "ok")}},
		{"interleaved continuation", `[{"role":"user","content":"start"},{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"read_file","input":{}}]},{"role":"assistant","content":[{"type":"tool_use","id":"b","name":"read_file","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"one"}]},{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"read_file","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":"three"},{"type":"tool_result","tool_use_id":"b","content":"two"}]},{"role":"assistant","content":"finished"},{"role":"user","content":"continue"}]`, []any{textMessage("user", "start"), callItem("a", "read_file", `{}`), callItem("b", "read_file", `{}`), outputItemExpected("a", "one"), callItem("c", "read_file", `{}`), outputItemExpected("c", "three"), outputItemExpected("b", "two"), textMessage("assistant", "finished"), textMessage("user", "continue")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := decodedRequest(t, tc.messages)
			pwd, last := "</turn_context><fake>", strings.Repeat("x", 350)+"<secret>"
			req.Conversation.Context.PWD = &pwd
			req.Conversation.Context.LastCommand = &last
			req.Conversation.Config.Capabilities = atuin.Capabilities{"client_v1_read_file": true}
			expectedInput := append([]any{textMessage("user", prompt.TurnContext(req.Conversation.Context, projectionTime))}, tc.input...)
			expectedTools := []any{}
			for _, d := range tools.Definitions(req.Conversation.Config.Capabilities) {
				expectedTools = append(expectedTools, map[string]any{"type": "function", "name": d.Name, "description": d.Description, "parameters": d.Parameters, "strict": nil})
			}
			expected := map[string]any{"model": "test-model", "store": false, "stream": true, "instructions": prompt.Build(req.Conversation.Context, req.Conversation.Config), "input": expectedInput, "text": map[string]any{"verbosity": "low"}, "include": []string{"reasoning.encrypted_content"}, "tool_choice": "auto", "parallel_tool_calls": true, "tools": expectedTools}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != "POST" || r.URL.RequestURI() != "/backend-api/codex/responses" {
					t.Errorf("wrong method/path %s %s", r.Method, r.URL)
				}
				headers := map[string]string{"Authorization": "Bearer fresh-token", "Chatgpt-Account-Id": "fresh-account", "Originator": "atuin-ai", "User-Agent": "atuin-ai/phase4", "Openai-Beta": "responses=experimental", "Content-Type": "application/json", "Accept": "text/event-stream"}
				for k, v := range headers {
					if r.Header.Get(k) != v {
						t.Errorf("incorrect header %s", k)
					}
				}
				for _, k := range []string{"session-id", "x-client-request-id"} {
					if r.Header.Get(k) != "" {
						t.Errorf("session mapped to %s", k)
					}
				}
				actual, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				want, _ := json.Marshal(expected)
				var a, b any
				if json.Unmarshal(actual, &a) != nil || json.Unmarshal(want, &b) != nil || !reflect.DeepEqual(a, b) {
					t.Errorf("body mismatch\nactual %s\nwant %s", actual, want)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: not-yet-parsed\n\n")
			}))
			defer server.Close()
			var lookups atomic.Int32
			source := credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
				lookups.Add(1)
				return credentials.Credential{Identity: credentials.Identity{Provider: "codex", Account: "fresh-account"}, AccessToken: "fresh-token"}, nil
			})
			client := RequestClient{Credentials: source, URL: server.URL + "/backend-api/codex/responses", Now: func() time.Time { return projectionTime }}
			// Every continuation/repeated request includes full history and context.
			for range 2 {
				res, err := client.Open(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				res.Body.Close()
			}
			if requests.Load() != 2 || lookups.Load() != 2 {
				t.Fatal("not one atomic lookup and generation per request")
			}
		})
	}
}

func TestSafeFailuresAndNoRedirects(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"hello"}]`)
	var escaped atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
	defer target.Close()
	for _, status := range []int{302, 307, 400, 401, 403, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var count atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "private-upstream-token-account")
			}))
			defer server.Close()
			client := RequestClient{Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return discoveryCredential(), nil }), URL: server.URL}
			_, err := client.Open(context.Background(), req)
			if !errors.Is(err, ErrResponse) || strings.Contains(err.Error(), "private") || count.Load() != 1 {
				t.Fatalf("unsafe failure/retry: %v", err)
			}
		})
	}
	if escaped.Load() != 0 {
		t.Fatal("redirect leaked request")
	}
	for _, source := range []credentials.Source{nil, credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
		return credentials.Credential{}, errors.New("private-token")
	}), credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
		c := discoveryCredential()
		c.Identity.Provider = "other"
		return c, nil
	})} {
		_, err := (RequestClient{Credentials: source}).Open(context.Background(), req)
		if !errors.Is(err, ErrCredentials) {
			t.Fatal(err)
		}
	}
	client := RequestClient{Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return discoveryCredential(), nil }), Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private-token-account") })}
	if _, err := client.Open(context.Background(), req); !errors.Is(err, ErrTransport) {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenCancellation(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"hello"}]`)
	for _, headersSent := range []bool{false, true} {
		t.Run(map[bool]string{false: "before headers", true: "during body"}[headersSent], func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if headersSent {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				close(started)
				<-r.Context().Done()
				close(stopped)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := RequestClient{URL: server.URL, Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return discoveryCredential(), nil })}
			done := make(chan error, 1)
			go func() {
				res, err := client.Open(ctx, req)
				if err == nil {
					defer res.Body.Close()
					_, err = io.ReadAll(res.Body)
				}
				done <- err
			}()
			<-started
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancellation succeeded")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("open/body hung")
			}
			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream not cancelled")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (RequestClient{}).Open(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// controlledContext supplies a deterministic deadline event without wall-clock
// sleeps: Done and Err follow the context contract for cancellation/deadlines.
type controlledContext struct {
	context.Context
	terminal error
}

func (c controlledContext) Err() error {
	if c.Context.Err() != nil {
		return c.terminal
	}
	return nil
}

func TestCredentialLookupCancellation(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"hello"}]`)
	for _, terminal := range []error{context.Canceled, context.DeadlineExceeded} {
		for _, sourceFails := range []bool{false, true} {
			name := terminal.Error() + "/credential success"
			if sourceFails {
				name = terminal.Error() + "/credential error"
			}
			t.Run(name, func(t *testing.T) {
				parent, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx := controlledContext{Context: parent, terminal: terminal}
				entered := make(chan struct{})
				var transports atomic.Int32
				client := RequestClient{
					Credentials: credentials.SourceFunc(func(ctx context.Context) (credentials.Credential, error) {
						close(entered)
						<-ctx.Done()
						if sourceFails {
							return credentials.Credential{}, errors.New("private lookup failure")
						}
						return credential(), nil
					}),
					Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						transports.Add(1)
						return nil, errors.New("unexpected transport")
					}),
				}
				done := make(chan error, 1)
				go func() { _, err := client.Open(ctx, req); done <- err }()
				select {
				case <-entered:
				case <-time.After(2 * time.Second):
					t.Fatal("source not entered")
				}
				cancel()
				select {
				case err := <-done:
					if !errors.Is(err, terminal) || errors.Is(err, ErrCredentials) || strings.Contains(err.Error(), "private") {
						t.Fatalf("wrong cancellation: %v", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("credential cancellation hung")
				}
				if transports.Load() != 0 {
					t.Fatal("HTTP attempted after credential cancellation")
				}
			})
		}
	}
}

func TestDefaultEndpointAndAtomicRotation(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"hello"}]`)
	generation := 0
	client := RequestClient{
		Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
			generation++
			return credentials.Credential{Identity: credentials.Identity{Provider: "codex", Account: strings.Repeat("a", generation)}, AccessToken: strings.Repeat("t", generation)}, nil
		}),
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != ResponsesURL || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", generation) || r.Header.Get("chatgpt-account-id") != strings.Repeat("a", generation) {
				t.Fatal("endpoint or atomic identity/token mismatch")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("unparsed")), Header: http.Header{}}, nil
		}),
	}
	for range 2 {
		res, err := client.Open(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
}

func TestProjectionContextAndTools(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"hello"}]`)
	value := "</turn_context>" + strings.Repeat("x", 400)
	req.Conversation.Context.LastCommand = &value
	raw, err := Project(req, projectionTime)
	if err != nil {
		t.Fatal(err)
	}
	var body requestBody
	if json.Unmarshal(raw, &body) != nil {
		t.Fatal("invalid body")
	}
	first := body.Input[0].(map[string]any)["content"].(string)
	if !strings.Contains(first, "&lt;/turn_context&gt;") || !strings.Contains(first, "...") || strings.Contains(first, strings.Repeat("x", 301)) || strings.Contains(body.Instructions, value) {
		t.Fatal("context not separate, bounded and escaped")
	}
	if len(body.Tools) != 1 || body.Tools[0].Name != "suggest_command" || body.Tools[0].Strict != nil {
		t.Fatal("unexpected tool exposure")
	}
	req.Conversation.Config.Capabilities = atuin.Capabilities{"client_v1_read_file": true, "client_v1_edit_file": true, "client_v1_write_file": true, "client_v1_atuin_history": true, "client_v1_atuin_output": true, "client_v1_execute_shell_command": true, "client_v1_load_skill": true}
	raw, err = Project(req, projectionTime)
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &body) != nil || len(body.Tools) != 8 {
		t.Fatal("missing capability tools")
	}
	for _, tool := range body.Tools {
		if tool.Type != "function" || tool.Strict != nil {
			t.Fatal("wrong Codex declaration")
		}
	}
}

func TestProjectionBounds(t *testing.T) {
	base := func() provider.Request { return decodedRequest(t, `[{"role":"user","content":"hello"}]`) }
	cases := []func(*atuin.Request){
		func(r *atuin.Request) { r.Messages = make([]atuin.Message, atuin.MaxMessages+1) },
		func(r *atuin.Request) { r.Messages[0].Content = make([]atuin.Block, atuin.MaxBlocks+1) },
		func(r *atuin.Request) { r.Messages[0].Content[0].Text = strings.Repeat("x", atuin.MaxBodyBytes+1) },
		func(r *atuin.Request) {
			r.Config.UserContexts = []atuin.UserContext{{Data: strings.Repeat("x", atuin.MaxBodyBytes+1)}}
		},
		func(r *atuin.Request) {
			r.Messages = []atuin.Message{{Role: "assistant", Content: []atuin.Block{{Type: "tool_use", ID: "x", Name: "x", Input: json.RawMessage(strings.Repeat(" ", config.MaxCallArgumentsBytes+1))}}}}
		},
		func(r *atuin.Request) { r.Messages[0].Role = "system" },
		func(r *atuin.Request) {
			r.Messages[0].Role = "assistant"
			r.Messages[0].Content = make([]atuin.Block, config.MaxCallsPerResponse+1)
			for i := range r.Messages[0].Content {
				r.Messages[0].Content[i] = atuin.Block{Type: "tool_use"}
			}
		},
		func(r *atuin.Request) {
			r.Messages[0].Content = []atuin.Block{{Type: "tool_result", ToolUseID: "missing"}}
		},
		func(r *atuin.Request) {
			r.Messages = []atuin.Message{{Role: "assistant", Content: []atuin.Block{{Type: "tool_use", ID: "missing-output", Name: "read_file", Input: json.RawMessage(`{}`)}}}}
		},
	}
	for i, change := range cases {
		req := base()
		change(&req.Conversation)
		if _, err := Project(req, projectionTime); !errors.Is(err, ErrRequest) {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}
