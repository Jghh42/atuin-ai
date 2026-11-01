package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/continuation"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
	"github.com/Jghh42/atuin-ai/internal/turn"
)

type integrationTransport func(*http.Request) (*http.Response, error)

func (f integrationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type preparedFixture struct {
	t         *testing.T
	path      string
	now       time.Time
	manager   *credentials.Manager
	store     *continuation.Store
	client    provider.Provider
	transport http.RoundTripper
	posts     int
	wrap      func(credentials.Storage) credentials.Storage
}

func newPreparedFixture(t *testing.T, respond func(*preparedFixture, map[string]any) string, wrap ...func(credentials.Storage) credentials.Storage) *preparedFixture {
	t.Helper()
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	f := &preparedFixture{t: t, path: filepath.Join(root, "credentials"), now: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
	if len(wrap) > 0 {
		f.wrap = wrap[0]
	}
	f.transport = integrationTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != ResponsesURL || r.Method != "POST" {
			t.Fatal("unexpected synthetic upstream request")
		}
		f.posts++
		body, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if json.Unmarshal(body, &v) != nil || v["store"] != false || v["stream"] != true {
			t.Fatal("wrong native request")
		}
		if _, ok := v["previous_response_id"]; ok {
			t.Fatal("provider session fallback")
		}
		if r.Header.Get("Chatgpt-Account-Id") != "synthetic-account" {
			t.Fatal("identity/header mismatch")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(respond(f, v)))}, nil
	})
	disk, e := credentials.OpenStore(f.path)
	if e != nil {
		t.Fatal(e)
	}
	claims, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "synthetic-account"}, "exp": f.now.Add(12 * time.Hour).Unix()})
	token := "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".c2ln"
	intent := credentials.Record{Schema: 1, Provider: "codex", Registration: credentials.ClientID, State: "unusable", Generation: 1, AuthEpoch: 1, SavedAt: f.now}
	active := intent
	active.State = "active"
	active.Account = "synthetic-account"
	active.Access = token
	active.Refresh = "synthetic-refresh"
	active.IssuedAt = f.now.Add(-time.Minute)
	active.ExpiresAt = f.now.Add(12 * time.Hour)
	if e = disk.Begin(context.Background(), intent); e != nil {
		t.Fatal(e)
	}
	if e = disk.Commit(context.Background(), active); e != nil {
		t.Fatal(e)
	}
	f.open(disk)
	t.Cleanup(func() { f.manager.Close() })
	return f
}
func (f *preparedFixture) open(disk *credentials.DiskStore) {
	f.t.Helper()
	dir, e := disk.OpenContinuationDirectory()
	if e != nil {
		f.t.Fatal(e)
	}
	s, e := continuation.NewStoreWithClock(dir, func() time.Time { return f.now })
	if e != nil {
		f.t.Fatal(e)
	}
	var storage credentials.Storage = disk
	if f.wrap != nil {
		storage = f.wrap(storage)
	}
	m, e := credentials.NewManager(storage, credentials.ManagerOptions{Dependent: s, OAuth: credentials.OAuthClient{Transport: f.transport, Now: func() time.Time { return f.now }}})
	if e != nil {
		f.t.Fatal(e)
	}
	f.manager = m
	f.store = s
	f.client = NewPrepared(RequestClient{Transport: f.transport, Now: func() time.Time { return f.now }}, m, s)
}
func (f *preparedFixture) restart() {
	f.t.Helper()
	if e := f.manager.Close(); e != nil {
		f.t.Fatal(e)
	}
	disk, e := credentials.OpenStore(f.path)
	if e != nil {
		f.t.Fatal(e)
	}
	f.open(disk)
}
func initialRequest() atuin.Request {
	return atuin.Request{SessionID: "00000000-0000-4000-8000-000000000001", Config: atuin.Config{ProviderModel: "synthetic-model", Capabilities: atuin.Capabilities{"client_v1_read_file": true}}, Messages: []atuin.Message{{Role: "user", Content: []atuin.Block{{Type: "text", Text: "synthetic request"}}}}}
}

type turnCapture struct {
	calls     []tools.Call
	text      string
	names     []string
	failures  []turn.FailureReport
	reasoning []turn.ReasoningReport
}

func runPrepared(t *testing.T, p provider.Provider, r atuin.Request) turnCapture {
	t.Helper()
	var out turnCapture
	turn.RunWithObservers(context.Background(), p, r, turn.SinkFunc(func(_ context.Context, e turn.Event) error {
		out.names = append(out.names, e.Name)
		if e.Name == "tool_call" {
			out.calls = append(out.calls, e.Data.(tools.Call))
		}
		if e.Name == "text" {
			b, _ := json.Marshal(e.Data)
			var v struct{ Content string }
			_ = json.Unmarshal(b, &v)
			out.text += v.Content
		}
		return nil
	}), func(f turn.FailureReport) { out.failures = append(out.failures, f) }, func(r turn.ReasoningReport) { out.reasoning = append(out.reasoning, r) })
	return out
}
func appendPublicHistory(r atuin.Request, c turnCapture, interleave, reverse bool) atuin.Request {
	text := strings.TrimLeftFunc(c.text, continuation.RustWhiteSpace)
	run := atuin.Message{Role: "assistant"}
	if text != "" {
		run.Content = append(run.Content, atuin.Block{Type: "text", Text: text})
	}
	result := func(call tools.Call) atuin.Message {
		return atuin.Message{Role: "user", Content: []atuin.Block{{Type: "tool_result", ToolUseID: call.ID, Content: "result-" + call.Name, IsError: interleave}}}
	}
	for _, call := range c.calls {
		run.Content = append(run.Content, atuin.Block{Type: "tool_use", ID: call.ID, Name: call.Name, Input: append(json.RawMessage(nil), call.Input...)})
		if interleave {
			r.Messages = append(r.Messages, run, result(call))
			run = atuin.Message{Role: "assistant"}
		}
	}
	if !interleave {
		r.Messages = append(r.Messages, run)
		for j := range c.calls {
			n := j
			if reverse {
				n = len(c.calls) - j - 1
			}
			r.Messages = append(r.Messages, result(c.calls[n]))
		}
	}
	return r
}
func TestPreparedReasoningReplayNonreasoningMiddleRestartAndOriginalOrder(t *testing.T) {
	for _, interleave := range []bool{true, false} {
		t.Run(fmt.Sprint(interleave), func(t *testing.T) {
			f := newPreparedFixture(t, func(f *preparedFixture, v map[string]any) string {
				input := v["input"].([]any)
				if f.posts == 1 {
					if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
						t.Fatal("unmarked Prepare eagerly created root key")
					}
					r := reasonOutput()
					r["content"] = nil
					r["status"] = nil
					r["summary"] = []any{reasonPart("summary_text", "PRIVATE-SUMMARY")}
					m := msg(" \t\u3000Checking \n")
					m["content"] = []any{reasonPart("output_text", " \t\u3000"), reasonPart("output_text", "Checking \n")}
					m["status"] = "completed"
					m["role"] = "assistant"
					return complete(r, m, fn("item-a", "backend-a", `{ "file_path":"synthetic-a", "extra":[0,-9223372036854775808,18446744073709551615] }`), fn("item-b", "backend-b", `{"file_path":"synthetic-b"}`))
				}
				// Every replay preserves original reasoning/function item IDs and call IDs,
				// original raw args, exact ordinary part boundaries/leading/trailing bytes.
				var types, outputs []string
				reasonCount := 0
				for _, entry := range input {
					item := entry.(map[string]any)
					if typ, ok := item["type"].(string); ok {
						types = append(types, typ)
						switch typ {
						case "reasoning":
							reasonCount++
							if item["id"] != "reason-1" || item["encrypted_content"] != "PRIVATE-ENCRYPTED" {
								t.Fatal("required state lost")
							}
							if _, ok := item["content"]; ok {
								t.Fatal("nullable output content replayed as invalid input null")
							}
							if _, ok := item["status"]; ok {
								t.Fatal("nullable output status replayed as invalid input null")
							}
						case "function_call":
							if item["call_id"] == "backend-a" {
								if item["id"] != "item-a" || item["arguments"] != `{ "file_path":"synthetic-a", "extra":[0,-9223372036854775808,18446744073709551615] }` {
									t.Fatal("original function identity/argument bytes lost")
								}
							}
						case "message":
							if item["id"] == "msg-1" {
								parts := item["content"].([]any)
								if len(parts) != 2 || parts[0].(map[string]any)["text"] != " \t\u3000" || parts[1].(map[string]any)["text"] != "Checking \n" {
									t.Fatal("text spans/trailing bytes lost")
								}
							}
						case "function_call_output":
							outputs = append(outputs, item["call_id"].(string))
						}
					}
				}
				if reasonCount != 1 {
					t.Fatal("original output omitted or replayed twice")
				}
				if len(types) < 5 || strings.Join(types[:5], ",") != "reasoning,message,function_call,function_call,function_call_output" {
					t.Fatal("whole original output not placed before actual results")
				}
				want := []string{"backend-b", "backend-a"}
				if interleave {
					want = []string{"backend-a", "backend-b"}
				}
				if f.posts == 3 {
					want = append(want, "backend-c")
				}
				if strings.Join(outputs, ",") != strings.Join(want, ",") {
					t.Fatal("result relative order changed")
				}
				if f.posts == 2 {
					m := msg("\u00a0Next \n")
					m["id"] = "message-middle"
					return complete(m, fn("item-c", "backend-c", `{"file_path":"synthetic-c"}`))
				}
				r := reasonOutput()
				r["id"] = "terminal-reason"
				r["encrypted_content"] = nil
				return complete(r, msg("finished"))
			})
			r := initialRequest()
			first := runPrepared(t, f.client, r)
			if len(first.calls) != 2 || len(first.failures) != 0 || len(first.reasoning) != 1 || first.reasoning[0].Category() != "reasoning_handled" {
				t.Fatal("first protected generation failed")
			}
			for _, call := range first.calls {
				if len(call.ID) != 90 || !continuation.Reserved(call.ID) {
					t.Fatal("protected opaque ID not issued")
				}
			}
			r = appendPublicHistory(r, first, interleave, true)
			pwd := "volatile-new-directory"
			r.Context.PWD = &pwd
			f.now = f.now.Add(time.Second)
			f.restart()
			middle := runPrepared(t, f.client, r)
			if len(middle.calls) != 1 || !continuation.Reserved(middle.calls[0].ID) || len(middle.failures) != 0 || len(middle.reasoning) != 1 || middle.reasoning[0].Category() != "reasoning_replayed" {
				t.Fatal("inherited nonreasoning generation not protected")
			}
			// A newer snapshot must survive removal of the old snapshot file.
			old := first.calls[0].ID[22:54] + ".sealed"
			if e := os.Remove(filepath.Join(f.path, "continuation-v1", old)); e != nil {
				t.Fatal(e)
			}
			r = appendPublicHistory(r, middle, false, false)
			f.now = f.now.Add(time.Second)
			f.restart()
			last := runPrepared(t, f.client, r)
			if last.text != "finished" || len(last.calls) != 0 || len(last.failures) != 0 || len(last.reasoning) != 1 || last.reasoning[0].Category() != "reasoning_replayed_and_handled" {
				t.Fatal("restart continuation terminal failed")
			}
			if f.posts != 3 {
				t.Fatal("generation retried")
			}
		})
	}
}
func TestPreparedInvalidHistoryRejectsBeforeGeneration(t *testing.T) {
	f := newPreparedFixture(t, func(_ *preparedFixture, _ map[string]any) string {
		return complete(reasonOutput(), fn("a", "backend-a", `{"file_path":"a"}`), fn("b", "backend-b", `{"file_path":"b"}`))
	})
	r := initialRequest()
	first := runPrepared(t, f.client, r)
	r = appendPublicHistory(r, first, true, false)
	cases := []struct {
		name   string
		change func(*atuin.Request)
	}{
		{"missing-sibling-pair", func(r *atuin.Request) { r.Messages = r.Messages[:3] }},
		{"duplicate-result", func(r *atuin.Request) { r.Messages = append(r.Messages, r.Messages[2]) }},
		{"model", func(r *atuin.Request) { r.Config.ProviderModel = "other" }},
		{"session", func(r *atuin.Request) { r.SessionID = "00000000-0000-4000-8000-000000000002" }},
		{"stable-environment", func(r *atuin.Request) { v := "different-os"; r.Context.OS = &v }},
		{"prefix", func(r *atuin.Request) { r.Messages[0].Content[0].Text = "changed" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := ownRequest(r)
			tc.change(&changed)
			before := f.posts
			got := runPrepared(t, f.client, changed)
			if f.posts != before || len(got.calls) != 0 || len(got.failures) != 1 || strings.Count(strings.Join(got.names, ","), "error") != 1 || len(got.reasoning) != 0 {
				t.Fatal("invalid protected history performed generation or release")
			}
		})
	}
}
func TestPreparedRequiredStateNumbersAndTerminalExceptions(t *testing.T) {
	for _, kind := range []string{"missing-state", "float-extra", "exponent-extra", "minus-zero", "reserved-provider", "text-terminal", "suggestion-terminal"} {
		t.Run(kind, func(t *testing.T) {
			f := newPreparedFixture(t, func(_ *preparedFixture, _ map[string]any) string {
				r := reasonOutput()
				arg := `{"file_path":"a"}`
				call := fn("f", "backend", arg)
				switch kind {
				case "missing-state":
					delete(r, "encrypted_content")
				case "float-extra":
					call["arguments"] = `{"file_path":"a","extra":[1.0]}`
				case "exponent-extra":
					call["arguments"] = `{"file_path":"a","extra":{"n":1e0}}`
				case "minus-zero":
					call["arguments"] = `{"file_path":"a","extra":-0}`
				case "reserved-provider":
					call["call_id"] = "aic1_literal"
				case "text-terminal":
					r["encrypted_content"] = nil
					return complete(r, msg("visible"))
				case "suggestion-terminal":
					r["encrypted_content"] = nil
					call["name"] = "suggest_command"
					call["arguments"] = `{"command":"rm -rf /synthetic","description":"synthetic","confidence":"high","danger":"low"}`
				}
				return complete(r, call)
			})
			got := runPrepared(t, f.client, initialRequest())
			terminal := strings.HasSuffix(kind, "terminal")
			if terminal {
				if len(got.failures) != 0 || len(got.reasoning) != 1 {
					t.Fatal("valid independent terminal rejected")
				}
				if kind == "suggestion-terminal" {
					if len(got.calls) != 1 || continuation.Reserved(got.calls[0].ID) || !strings.Contains(string(got.calls[0].Input), `"danger":"high"`) {
						t.Fatal("terminal suggestion safety/ID changed")
					}
				}
			} else {
				if len(got.calls) != 0 || len(got.failures) != 1 || len(got.reasoning) != 0 {
					t.Fatal("invalid executable continuation escaped")
				}
			}
			if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
				t.Fatal("invalid/terminal generation created private index")
			}
		})
	}
}
func TestRawEntryPointsCannotBypassMarkedHistory(t *testing.T) {
	r := initialRequest()
	r.Messages = append(r.Messages, atuin.Message{Role: "assistant", Content: []atuin.Block{{Type: "tool_use", ID: "aic1_bad", Name: "read_file", Input: []byte(`{"file_path":"x"}`)}}}, atuin.Message{Role: "user", Content: []atuin.Block{{Type: "tool_result", ToolUseID: "aic1_bad", Content: "x"}}})
	if _, e := Project(provider.Request{Conversation: r}, time.Now()); e == nil {
		t.Fatal("raw projection accepted marker")
	}
	posts := 0
	c := RequestClient{Transport: integrationTransport(func(*http.Request) (*http.Response, error) { posts++; return nil, ErrTransport })}
	if _, e := c.Open(context.Background(), provider.Request{Conversation: r}); e == nil {
		t.Fatal("raw Open accepted marker")
	}
	if _, e := c.Stream(context.Background(), provider.Request{Conversation: r}, func(provider.Event) error { return nil }); e == nil {
		t.Fatal("raw Stream accepted marker")
	}
	if posts != 0 {
		t.Fatal("marker reached generation transport")
	}
}
func TestPrivatePreparedPresentations(t *testing.T) {
	f := newPreparedFixture(t, func(_ *preparedFixture, _ map[string]any) string {
		return complete(reasonOutput(), fn("f", "backend", `{"file_path":"synthetic"}`))
	})
	p, e := f.client.(provider.PreparedProvider).Prepare(context.Background(), provider.Request{Conversation: initialRequest()})
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	_, e = p.Stream(context.Background(), func(provider.Event) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	values := []any{p, f.client, p.(*preparedGeneration).candidate, p.(*preparedGeneration).candidate.capture, &reasoning{reasoningPayload: &reasoningPayload{id: "PRIVATE-MARKER", encrypted: "PRIVATE-MARKER", summary: []string{"PRIVATE-MARKER"}}}, &rawReasoning{rawReasoningPayload: &rawReasoningPayload{data: []byte("PRIVATE-MARKER")}}}
	for _, v := range values {
		for _, nested := range []any{v, []any{v}, map[string]any{"payload": v}, struct{ Payload any }{v}} {
			for _, verb := range []string{"%v", "%#v", "%+v", "%w", "%+w", "%#w", "%s", "%q", "%d", "%p", "%#p", "%T"} {
				s := fmt.Sprintf(verb, nested)
				if strings.Contains(s, "PRIVATE-") || strings.Contains(s, "synthetic-account") || strings.Contains(s, "PANIC") {
					t.Fatal("private prepared fmt leak")
				}
			}
			b, e := json.Marshal(nested)
			if e != nil || bytes.Contains(b, []byte("PRIVATE-")) {
				t.Fatal("private prepared JSON leak")
			}
			var buf bytes.Buffer
			slog.New(slog.NewJSONHandler(&buf, nil)).Info("safe", "payload", nested)
			if strings.Contains(buf.String(), "PRIVATE-") || strings.Contains(buf.String(), "panicked") {
				t.Fatal("private prepared slog leak")
			}
		}
	}
}
