package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

func TestReviewExecutionMetadataAllShapesBeforeKeyOrTools(t *testing.T) {
	for _, row := range []struct {
		field string
		value any
	}{{"caller", map[string]any{"type": "program", "caller_id": "private"}}, {"caller", map[string]any{"type": "direct", "caller_id": "unexpected"}}, {"caller", map[string]any{}}, {"caller", []any{}}, {"caller", true}, {"CALLER", map[string]any{"type": "program"}}, {"async", true}, {"async", "false"}, {"async", 0}, {"ASYNC", true}} {
		for _, where := range []string{"added", "done", "final"} {
			for _, reasoning := range []bool{false, true} {
				f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
					item := fn("f", "backend", `{"file_path":"owned"}`)
					item[row.field] = row.value
					wire := ""
					if where != "final" {
						wire = frame("response.output_item."+where, map[string]any{"output_index": 0, "item": item})
						item = fn("f", "backend", `{"file_path":"owned"}`)
					}
					out := []map[string]any{item}
					if reasoning {
						out = append(out, reasonOutput())
					}
					return wire + complete(out...)
				})
				result := runPrepared(t, f.client, initialRequest())
				if len(result.calls) != 0 || len(result.failures) != 1 {
					t.Fatal("unsupported execution metadata escaped")
				}
				if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
					t.Fatal("unsupported execution created root key")
				}
			}
		}
	}
}
func TestReviewDirectMetadataPresenceRoundTrip(t *testing.T) {
	for _, caller := range []any{"omitted", nil, map[string]any{"type": "direct"}} {
		for _, async := range []any{"omitted", nil, false} {
			f := newPreparedFixture(t, func(f *preparedFixture, body map[string]any) string {
				if f.posts == 1 {
					item := fn("f", "backend", `{"file_path":"owned"}`)
					if caller != "omitted" {
						item["caller"] = caller
					}
					if async != "omitted" {
						item["async"] = async
					}
					return complete(reasonOutput(), schemaMessage("visible"), item)
				}
				for _, row := range body["input"].([]any) {
					item := row.(map[string]any)
					if item["type"] == "function_call" {
						v, ok := item["caller"]
						if caller == "omitted" {
							if ok {
								t.Error("omitted caller invented")
							}
						} else if caller == nil {
							if !ok || v != nil {
								t.Error("null caller lost")
							}
						} else if !ok || v.(map[string]any)["type"] != "direct" {
							t.Error("direct caller lost")
						}
						value, present := item["async"]
						if async == false {
							if !present || value != false {
								t.Error("false async lost")
							}
						} else if present {
							t.Error("omitted/null async must not produce input null")
						}
					}
				}
				return complete(schemaMessage("terminal"))
			})
			r := initialRequest()
			first := runPrepared(t, f.client, r)
			if len(first.calls) != 1 {
				t.Fatal("supported direct metadata failed")
			}
			runPrepared(t, f.client, appendPublicHistory(r, first, false, false))
		}
	}
}
func TestReviewMessageMetadataPresenceAndUnsupportedSubset(t *testing.T) {
	for _, key := range []string{"status", "role", "annotations", "logprobs", "phase"} {
		for _, value := range []any{"omitted", nil, 7, []any{map[string]any{"type": "unknown"}}} {
			f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
				m := schemaMessage("visible")
				target := m
				if key == "annotations" || key == "logprobs" {
					target = m["content"].([]any)[0].(map[string]any)
				}
				if value == "omitted" {
					delete(target, key)
				} else {
					target[key] = value
				}
				return complete(reasonOutput(), m, fn("f", "backend", `{"file_path":"owned"}`))
			})
			result := runPrepared(t, f.client, initialRequest())
			allowed := key == "logprobs" && (value == "omitted" || value == nil) || key == "phase" && (value == "omitted" || value == nil)
			if allowed {
				if len(result.calls) != 1 {
					t.Fatal("supported optional presence rejected")
				}
			} else if len(result.calls) != 0 || len(result.failures) == 0 {
				t.Fatal("unsupported required metadata published")
			}
		}
	}
}
func TestReviewOrdinaryCheckpointValidProgressAndConflicts(t *testing.T) {
	for _, kind := range []string{"message", "function"} {
		added := schemaMessage("visible")
		if kind == "function" {
			added = fn("f", "backend", `{"file_path":"owned"}`)
		}
		added["status"] = "in_progress"
		done := map[string]any{}
		for k, v := range added {
			done[k] = v
		}
		done["status"] = "completed"
		wire := frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + frame("response.output_item.done", map[string]any{"output_index": 0, "item": done}) + complete(done)
		if _, _, e := parsedPrepared(wire); e != nil {
			t.Fatal("provisional progress transition failed")
		}
	}
	for _, field := range []string{"role", "phase", "annotations", "logprobs", "caller", "async"} {
		item := schemaMessage("visible")
		if field == "caller" || field == "async" {
			item = fn("f", "backend", `{"file_path":"owned"}`)
		}
		final := map[string]any{}
		for k, v := range item {
			final[k] = v
		}
		switch field {
		case "role":
			delete(final, "role")
		case "phase":
			final[field] = nil
		case "annotations", "logprobs":
			part := map[string]any{"type": "output_text", "text": "visible", "annotations": []any{}}
			if field == "annotations" {
				delete(part, field)
			} else {
				part[field] = nil
			}
			final["content"] = []any{part}
		case "caller":
			item[field] = map[string]any{"type": "direct"}
		case "async":
			item[field] = false
		}
		wire := frame("response.output_item.done", map[string]any{"output_index": 0, "item": item}) + complete(final, reasonOutput())
		events, _, e := parsedPrepared(wire)
		if e == nil {
			t.Fatal("authoritative metadata changed/omitted without rejection")
		}
		assertNoCalls(t, events)
	}
}
func FuzzPreparedReasoningStream(f *testing.F) {
	f.Add([]byte(complete(reasonOutput(), schemaMessage("visible"), fn("f", "backend", `{"file_path":"owned"}`))))
	f.Add([]byte(reasoningAdded(reasonOutput())))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 2<<20 {
			return
		}
		_, c, _ := parseConfigured(&oneByteReader{strings.NewReader(string(raw))}, func(provider.Event) error { return nil }, true)
		if c != nil {
			c.close()
		}
	})
}
