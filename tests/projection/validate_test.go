package projection

import (
	"encoding/json"
	"strings"
	"testing"
)

const complexAtuin = `{"config":{"capabilities":["client_v1_write_file","client_v1_read_file"]},"messages":[{"role":"user","content":"question"},{"role":"assistant","content":[{"type":"text","text":"before"},{"type":"tool_use","id":"a","name":"read_file","input":{"file_path":"a"}},{"type":"tool_use","id":"b","name":"write_file","input":{"file_path":"b","content":"B"}},{"type":"text","text":"after"}]},{"role":"user","content":[{"type":"text","text":"u1"},{"type":"tool_result","tool_use_id":"b","content":"failed","is_error":true},{"type":"text","text":"u2"},{"type":"tool_result","tool_use_id":"a","content":"A"},{"type":"text","text":"u3"}]}]}`
const nativeItems = `[{"role":"user","content":"<turn_context><current_date_utc>2026-10-16</current_date_utc></turn_context>"},{"role":"user","content":"question"},{"role":"assistant","content":"before"},{"type":"function_call","call_id":"a","name":"read_file","arguments":"{\"file_path\":\"a\"}"},{"type":"function_call","call_id":"b","name":"write_file","arguments":"{\"file_path\":\"b\",\"content\":\"B\"}"},{"role":"assistant","content":"after"},{"role":"user","content":"u1"},{"type":"function_call_output","call_id":"b","output":"Tool execution failed: failed"},{"role":"user","content":"u2"},{"type":"function_call_output","call_id":"a","output":"A"},{"role":"user","content":"u3"}]`
const ccItems = `[{"role":"system","content":[{"type":"text","text":"independent system prose"}]},{"role":"user","content":[{"type":"text","text":"<turn_context><current_date_utc>October 16, 2026</current_date_utc></turn_context>"}]},{"role":"user","content":[{"type":"text","text":"question"}]},{"role":"assistant","content":[{"type":"text","text":"before"},{"type":"text","text":"after"}],"tool_calls":[{"id":"a","type":"function","function":{"name":"read_file","arguments":"{\"file_path\":\"a\"}"}},{"id":"b","type":"function","function":{"name":"write_file","arguments":"{\"content\":\"B\",\"file_path\":\"b\"}"}}]},{"role":"user","content":[{"type":"text","text":"u1"}]},{"role":"tool","tool_call_id":"b","content":"failed"},{"role":"user","content":[{"type":"text","text":"u2"}]},{"role":"tool","tool_call_id":"a","content":"A"},{"role":"user","content":[{"type":"text","text":"u3"}]}]`

func independentWire(t *testing.T, dialect string) map[string]any {
	t.Helper()
	var schemas map[string]any
	if decode([]byte(schemaJSON), &schemas) != nil {
		t.Fatal("schema literal")
	}
	tools := []any{}
	for _, name := range []string{"suggest_command", "read_file", "write_file"} {
		inner := map[string]any{"name": name, "description": "independent prose", "parameters": schemas[name]}
		if dialect == "responses" {
			inner["type"] = "function"
			inner["strict"] = nil
			tools = append(tools, inner)
		} else {
			tools = append(tools, map[string]any{"type": "function", "function": inner})
		}
	}
	v := map[string]any{"model": Model, "stream": true, "tools": tools}
	var items []any
	if dialect == "responses" {
		_ = decode([]byte(nativeItems), &items)
		v["input"] = items
		v["store"] = false
		v["instructions"] = "independent instructions"
	} else {
		_ = decode([]byte(ccItems), &items)
		v["messages"] = items
	}
	return v
}
func TestSelectedLunaRequiresExactProjection(t *testing.T) {
	for _, dialect := range []string{"responses", "chat-completions"} {
		raw := strings.Replace(complexAtuin, `"config":{`, `"config":{"model":"gpt-6-luna",`, 1)
		v := independentWire(t, dialect)
		v["model"] = "gpt-6-luna"
		b, _ := json.Marshal(v)
		if err := Validate(dialect, []byte(raw), b); err != nil {
			t.Fatal(err)
		}
		for _, wrong := range []string{Model, "invented"} {
			v["model"] = wrong
			b, _ := json.Marshal(v)
			if Validate(dialect, []byte(raw), b) == nil {
				t.Fatal("chosen model not enforced")
			}
		}
	}
}

func TestSemanticProjectionMutations(t *testing.T) {
	for _, dialect := range []string{"responses", "chat-completions"} {
		t.Run(dialect, func(t *testing.T) {
			good := independentWire(t, dialect)
			b, _ := json.Marshal(good)
			if e := Validate(dialect, []byte(complexAtuin), b); e != nil {
				t.Fatal(e)
			}
			itemsKey := "input"
			if dialect == "chat-completions" {
				itemsKey = "messages"
			}
			inner := func(w map[string]any, index int) map[string]any {
				v := w["tools"].([]any)[index].(map[string]any)
				if dialect == "chat-completions" {
					return v["function"].(map[string]any)
				}
				return v
			}
			mutations := map[string]func(map[string]any){
				"wrong-model":   func(v map[string]any) { v["model"] = "wrong" },
				"missing-tools": func(v map[string]any) { delete(v, "tools") },
				"tool-order":    func(v map[string]any) { a := v["tools"].([]any); a[1], a[2] = a[2], a[1] },
				"constraint": func(v map[string]any) {
					p := inner(v, 1)["parameters"].(map[string]any)
					p["properties"].(map[string]any)["limit"].(map[string]any)["maximum"] = json.Number("999")
				},
				"required": func(v map[string]any) { inner(v, 1)["parameters"].(map[string]any)["required"] = []any{} },
				"description-parameter-removed": func(v map[string]any) {
					delete(inner(v, 0)["parameters"].(map[string]any)["properties"].(map[string]any), "description")
				},
				"dropped-history":   func(v map[string]any) { a := v[itemsKey].([]any); v[itemsKey] = a[:len(a)-1] },
				"reordered-history": func(v map[string]any) { a := v[itemsKey].([]any); a[len(a)-1], a[len(a)-2] = a[len(a)-2], a[len(a)-1] },
				"correlation": func(v map[string]any) {
					a := v[itemsKey].([]any)
					last := a[len(a)-2].(map[string]any)
					if dialect == "responses" {
						last["call_id"] = "wrong"
					} else {
						last["tool_call_id"] = "wrong"
					}
				},
				"missing-stream": func(v map[string]any) { delete(v, "stream") },
			}
			for name, change := range mutations {
				t.Run(name, func(t *testing.T) {
					v := independentWire(t, dialect)
					change(v)
					b, _ := json.Marshal(v)
					if Validate(dialect, []byte(complexAtuin), b) == nil {
						t.Fatal("canned fake could hide outbound mutation")
					}
				})
			}
			inner(good, 1)["parameters"].(map[string]any)["description"] = "different schema prose"
			inner(good, 1)["description"] = "different tool prose"
			b, _ = json.Marshal(good)
			if e := Validate(dialect, []byte(complexAtuin), b); e != nil {
				t.Fatal("prose-only difference rejected", e)
			}
			if Validate(dialect, []byte(complexAtuin), append(b, []byte(` {}`)...)) == nil {
				t.Fatal("trailing generation JSON accepted")
			}
		})
	}
}
func TestLegacyAndMultipleContinuationHistory(t *testing.T) {
	raw := complexAtuin
	// Multiple call/result pairs and out-of-order parallel results are covered by
	// the complex literal. Separately prove legacy top-level calls are appended.
	var req map[string]any
	_ = decode([]byte(raw), &req)
	messages := req["messages"].([]any)
	assistant := messages[1].(map[string]any)
	blocks := assistant["content"].([]any)
	assistant["content"] = []any{blocks[0], blocks[3]}
	assistant["tool_calls"] = []any{map[string]any{"id": "a", "name": "read_file", "input": map[string]any{"file_path": "a"}}, map[string]any{"id": "b", "name": "write_file", "input": map[string]any{"file_path": "b", "content": "B"}}}
	list, e := normalized(req)
	if e != nil {
		t.Fatal(e)
	}
	if len(list[1].Blocks) != 4 || list[1].Blocks[2].ID != "a" || list[1].Blocks[3].ID != "b" {
		t.Fatal("legacy calls not appended in order")
	}
	rawBytes, _ := json.Marshal(req)
	for _, dialect := range []string{"responses", "chat-completions"} {
		wire := independentWire(t, dialect)
		if dialect == "responses" {
			items := wire["input"].([]any)
			ordered := append([]any{}, items[:2]...)
			ordered = append(ordered, items[2], items[5], items[3], items[4])
			ordered = append(ordered, items[6:]...)
			wire["input"] = ordered
		}
		b, _ := json.Marshal(wire)
		if e := Validate(dialect, rawBytes, b); e != nil {
			t.Fatal("legacy replay", e)
		}
	}
}

func TestSuggestionHistoryHasDialectSpecificNonExecutionWitness(t *testing.T) {
	raw := map[string]any{}
	_ = decode([]byte(`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"s","name":"suggest_command","input":{"command":"echo synthetic"}}]},{"role":"user","content":"next"}]}`), &raw)
	messages, e := normalized(raw)
	if e != nil {
		t.Fatal(e)
	}
	native, cc := nativeHistory(messages), ccHistory(messages)
	if len(native) != 2 || native[0].(map[string]any)["role"] != "assistant" {
		t.Fatal("native suggestion history became executable")
	}
	if len(cc) != 3 || cc[1].(map[string]any)["content"] != `{"success": true}` || cc[1].(map[string]any)["tool_call_id"] != "s" {
		t.Fatal("reference acknowledgment changed")
	}
}
