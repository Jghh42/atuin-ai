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
)

func TestReviewRequiredDoneHTTPBoundaryZeroToolFrames(t *testing.T) {
	for _, mode := range []string{"content-missing", "content-null", "text-missing", "text-null", "part-text-missing", "status-reset"} {
		t.Run(mode, func(t *testing.T) {
			f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
				done := schemaMessage("")
				done["content"] = []any{}
				final := schemaMessage("")
				final["content"] = []any{}
				wire := ""
				switch mode {
				case "content-missing":
					delete(done, "content")
				case "content-null":
					done["content"] = nil
				case "status-reset":
					delete(done, "role")
					done["status"] = "unsupported"
					return frame("response.output_item.added", map[string]any{"output_index": 0, "item": done}) + complete(final, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
				default:
					part := map[string]any{"type": "output_text", "annotations": []any{}}
					if mode == "text-null" {
						part["text"] = nil
					}
					final["content"] = []any{map[string]any{"type": "output_text", "text": "", "annotations": []any{}}}
					done["content"] = []any{part}
					if mode == "part-text-missing" {
						added := schemaMessage("")
						added["content"] = []any{}
						wire = frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + frame("response.content_part.done", map[string]any{"output_index": 0, "item_id": "schema-message", "content_index": 0, "part": part})
					}
				}
				if wire == "" {
					wire = frame("response.output_item.done", map[string]any{"output_index": 0, "item": done})
				}
				return wire + complete(final, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
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
			stopped := make(chan error, 1)
			go func() { stopped <- server.Serve(ctx, listener) }()
			defer func() {
				cancel()
				select {
				case e := <-stopped:
					if e != nil {
						t.Error(e)
					}
				case <-time.After(6 * time.Second):
					t.Error("owned HTTP server cleanup did not join")
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
				t.Fatal("required metadata failure escaped real HTTP terminal/tool boundary")
			}
			if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
				t.Fatal("invalid DONE reached private key commit")
			}
		})
	}
}
