package codex

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/httpserver"
	"github.com/Jghh42/atuin-ai/internal/provider"
)

func nullRequiredWire(field, stage string) string {
	added := schemaMessage("")
	added["status"] = "in_progress"
	done := schemaMessage("")
	final := schemaMessage("visible")
	item := added
	if stage != "added" {
		item = done
	}
	if strings.HasPrefix(field, "part.") {
		part := item["content"].([]any)[0].(map[string]any)
		part[strings.TrimPrefix(field, "part.")] = nil
	} else {
		item[field] = nil
	}
	switch stage {
	case "added":
		return frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + complete(final, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
	case "done":
		return frame("response.output_item.done", map[string]any{"output_index": 0, "item": done}) + complete(final, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
	default:
		return complete(done, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
	}
}
func TestReviewSuppliedNullRequiredMessageNeverRepaired(t *testing.T) {
	for _, field := range []string{"status", "content", "role", "id", "type", "part.text", "part.type", "part.annotations"} {
		for _, stage := range []string{"added", "done", "final"} {
			t.Run(field+"/"+stage, func(t *testing.T) {
				f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string { return nullRequiredWire(field, stage) })
				result := runPrepared(t, f.client, initialRequest())
				if len(result.calls) != 0 || len(result.failures) != 1 || f.posts != 1 {
					t.Fatal("supplied null required field repaired by later metadata")
				}
				if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
					t.Fatal("supplied-null metadata committed key")
				}
			})
		}
	}
}
func TestReviewSuppliedNullRequiredHTTPZeroToolFrames(t *testing.T) {
	for _, field := range []string{"status", "content", "function.arguments"} {
		t.Run(field, func(t *testing.T) {
			f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
				if field == "function.arguments" {
					return nullFunctionRequiredWire("arguments", "added")
				}
				return nullRequiredWire(field, "added")
			})
			server, e := httpserver.NewWithRuntime(config.Default(), nil, httpserver.Runtime{Credentials: f.manager, Catalog: catalog.New(f.manager, ""), Provider: f.client})
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
			defer func() {
				cancel()
				select {
				case e := <-done:
					if e != nil {
						t.Error(e)
					}
				case <-time.After(6 * time.Second):
					t.Error("owned HTTP cleanup did not join")
				}
			}()
			client := http.Client{Timeout: 5 * time.Second}
			response, e := client.Post("http://"+listener.Addr().String()+"/api/cli/chat", "application/json", strings.NewReader(`{"session_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","messages":[{"role":"user","content":"owned"}],"config":{"capabilities":["client_v1_read_file"]}}`))
			if e != nil {
				t.Fatal(e)
			}
			body, e := io.ReadAll(response.Body)
			response.Body.Close()
			if e != nil || response.StatusCode != 200 || strings.Count(string(body), "event: error\n") != 1 || strings.Contains(string(body), "event: tool_call") || strings.Contains(string(body), "event: done") || f.posts != 1 {
				t.Fatal("supplied-null required metadata escaped HTTP boundary")
			}
			if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
				t.Fatal("invalid metadata reached private commit")
			}
		})
	}
}
func TestReviewSuppliedNullRequiredPartCheckpoints(t *testing.T) {
	for _, field := range []string{"text", "annotations", "type"} {
		for _, stage := range []string{"added", "done"} {
			t.Run(field+"/"+stage, func(t *testing.T) {
				f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
					added := schemaMessage("")
					added["status"] = "in_progress"
					added["content"] = []any{}
					part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}}
					part[field] = nil
					return frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + frame("response.content_part."+stage, map[string]any{"output_index": 0, "item_id": "schema-message", "content_index": 0, "part": part}) + complete(schemaMessage("visible"), reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
				})
				result := runPrepared(t, f.client, initialRequest())
				if len(result.calls) != 0 || len(result.failures) != 1 || f.posts != 1 {
					t.Fatal("null required part repaired")
				}
				if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
					t.Fatal("invalid part committed key")
				}
			})
		}
	}
}
func nullFunctionRequiredWire(field, stage string) string {
	item := fn("f", "backend", "")
	item[field] = nil
	wire := ""
	final := fn("f", "backend", `{"file_path":"owned"}`)
	if stage == "final" {
		final = item
	} else {
		wire = frame("response.output_item."+stage, map[string]any{"output_index": 2, "item": item})
	}
	return wire + complete(schemaMessage("visible"), reasonOutput(), final)
}
func TestReviewNullRequiredFunctionNeverRepaired(t *testing.T) {
	for _, field := range []string{"id", "type", "name", "call_id", "arguments"} {
		for _, stage := range []string{"added", "done", "final"} {
			t.Run(field+"/"+stage, func(t *testing.T) {
				f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string { return nullFunctionRequiredWire(field, stage) })
				result := runPrepared(t, f.client, initialRequest())
				if len(result.calls) != 0 || len(result.failures) != 1 || f.posts != 1 {
					t.Fatal("supplied null required function field repaired")
				}
				if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
					t.Fatal("invalid function metadata committed key")
				}
			})
		}
	}
}
func TestReviewApprovedNullableMetadataControls(t *testing.T) {
	f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
		message := schemaMessage("visible")
		message["phase"] = nil
		message["content"].([]any)[0].(map[string]any)["logprobs"] = nil
		function := fn("f", "backend", `{"file_path":"owned"}`)
		function["status"] = nil
		function["caller"] = nil
		function["async"] = nil
		reason := reasonOutput()
		reason["status"] = nil
		reason["content"] = nil
		return frame("response.output_item.added", map[string]any{"output_index": 0, "item": message}) + frame("response.output_item.done", map[string]any{"output_index": 0, "item": message}) + complete(message, reason, function)
	})
	result := runPrepared(t, f.client, initialRequest())
	if len(result.calls) != 1 || len(result.failures) != 0 || f.posts != 1 {
		t.Fatal("approved optional nullable metadata rejected")
	}
	terminal := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
		reason := reasonOutput()
		reason["status"] = nil
		reason["content"] = nil
		reason["encrypted_content"] = nil
		return complete(reason, schemaMessage("visible"))
	})
	result = runPrepared(t, terminal.client, initialRequest())
	if len(result.calls) != 0 || len(result.failures) != 0 || terminal.posts != 1 {
		t.Fatal("nullable reasoning fields rejected on independent terminal text")
	}
	if _, e := os.Stat(filepath.Join(terminal.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
		t.Fatal("independent terminal created key")
	}
}
func TestReviewNullRequiredReasoningNeverRepaired(t *testing.T) {
	for _, field := range []string{"id", "type", "summary", "summary.type", "summary.text"} {
		t.Run(field, func(t *testing.T) {
			f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
				reason := reasonOutput()
				if strings.HasPrefix(field, "summary.") {
					part := map[string]any{"type": "summary_text", "text": ""}
					part[strings.TrimPrefix(field, "summary.")] = nil
					reason["summary"] = []any{part}
				} else {
					reason[field] = nil
				}
				return reasoningAdded(reason) + complete(schemaMessage("visible"), reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
			})
			result := runPrepared(t, f.client, initialRequest())
			if len(result.calls) != 0 || len(result.failures) != 1 || f.posts != 1 {
				t.Fatal("null required reasoning repaired")
			}
		})
	}
}
func TestReviewOmittedFunctionArgumentsMayComplete(t *testing.T) {
	f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
		added := fn("f", "backend", "")
		delete(added, "arguments")
		return frame("response.output_item.added", map[string]any{"output_index": 2, "item": added}) + complete(schemaMessage("visible"), reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
	})
	result := runPrepared(t, f.client, initialRequest())
	if len(result.calls) != 1 || len(result.failures) != 0 || f.posts != 1 {
		t.Fatal("permitted provisional argument omission rejected")
	}
}
func TestReviewRawLegacyNullArgumentPrefixUnchanged(t *testing.T) {
	added := fn("f", "backend", "")
	added["arguments"] = nil
	wire := frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + complete(fn("f", "backend", `{"file_path":"owned"}`))
	if _, e := parseStream(strings.NewReader(wire), func(event provider.Event) error { return nil }); e != nil {
		t.Fatal("legacy ordinary null argument prefix changed")
	}
}
func TestReviewOmittedProvisionalRequiredMayComplete(t *testing.T) {
	for _, field := range []string{"status", "content", "role"} {
		f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
			added := schemaMessage("")
			added["status"] = "in_progress"
			delete(added, field)
			final := schemaMessage("visible")
			return frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + frame("response.output_item.done", map[string]any{"output_index": 0, "item": final}) + complete(final, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
		})
		result := runPrepared(t, f.client, initialRequest())
		if len(result.calls) != 1 || len(result.failures) != 0 {
			t.Fatal("allowed missing provisional field incorrectly treated as null")
		}
	}
}
