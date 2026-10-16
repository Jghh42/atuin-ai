package codex

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

func assertNoCallsFailure(t *testing.T, input string) {
	t.Helper()
	events, _, err := collect(input)
	if err == nil {
		t.Fatal("invalid stream succeeded")
	}
	for _, event := range events {
		if event.Kind == provider.ToolCalls {
			t.Fatal("tool batch escaped failed reconciliation")
		}
	}
}
func emptyMessage(id string) map[string]any {
	return map[string]any{"type": "message", "id": id, "content": []any{}}
}
func indexedText(index int, text string) string {
	return frame("response.output_text.delta", map[string]any{"output_index": 0, "content_index": index, "item_id": "msg-1", "delta": text})
}
func announcedPart(output, index int, id string) string {
	return frame("response.content_part.added", map[string]any{"output_index": output, "content_index": index, "item_id": id, "part": map[string]any{"type": "output_text", "text": ""}})
}

func TestSparseObservedSlotsCannotDisappear(t *testing.T) {
	for _, index := range []int{1, 127} {
		t.Run(fmt.Sprint("output-", index), func(t *testing.T) {
			assertNoCallsFailure(t, added(index, fn("item-a", "call-a", ""))+complete(fn("item-b", "call-b", "{}")))
		})
		t.Run(fmt.Sprint("content-", index), func(t *testing.T) {
			// Map cardinality is one, but its sole content index is not in the final
			// one-element array. A different legitimate tool must NOT escape.
			observed := added(0, emptyMessage("msg-1")) + indexedText(index, "unfinished")
			assertNoCallsFailure(t, observed+complete(msg(""), fn("item-b", "call-b", "{}")))
			// The same invariant applies to output_item.done before completion.
			assertNoCallsFailure(t, observed+frame("response.output_item.done", map[string]any{"output_index": 0, "item": msg("")})+complete(msg(""), fn("item-b", "call-b", "{}")))
		})
	}
	// Sparse/interleaved indices remain supported if all appear in final output.
	stream := added(1, fn("item-b", "call-b", "")) + added(0, fn("item-a", "call-a", "")) + complete(fn("item-a", "call-a", "{}"), fn("item-b", "call-b", "{}"))
	events, _, err := collect(stream)
	if err != nil || len(events) != 1 || len(events[0].Calls) != 2 {
		t.Fatalf("%+v %v", events, err)
	}
}

func TestUnsupportedExecutionChannelsWithoutFinalItems(t *testing.T) {
	for _, family := range []string{"web_search_call", "file_search_call", "code_interpreter_call", "image_generation_call", "computer_call", "local_shell_call", "shell_call", "apply_patch_call", "mcp_call", "mcp_list_tools", "mcp_approval_request", "custom_tool_call", "future_execution_call", "image_generation.partial_image", "computer.action", "shell.command", "apply_patch.operation", "custom.input"} {
		for _, state := range []string{"in_progress", "completed"} {
			t.Run(family+"."+state, func(t *testing.T) {
				events, _, err := collect(frame("response."+family+"."+state, nil) + complete(fn("i", "c", "{}")))
				if !errors.Is(err, ErrUnsupported) || len(events) != 0 {
					t.Fatalf("unsupported execution accepted: %+v %v", events, err)
				}
			})
		}
	}
	// Truly additive metadata is still compatible.
	if _, _, err := collect(frame("response.future_metadata", map[string]any{"hello": "future"}) + complete()); err != nil {
		t.Fatal(err)
	}
}

func TestResponseIdentityPolicy(t *testing.T) {
	lifecycle := func(kind string, id any) string {
		return frame("response."+kind, map[string]any{"response": map[string]any{"id": id, "status": "in_progress"}})
	}
	terminal := func(id any) string {
		return frame("response.completed", map[string]any{"response": map[string]any{"id": id, "status": "completed", "output": []any{fn("i", "c", "{}")}}})
	}
	for name, stream := range map[string]string{
		"created mismatch":                  lifecycle("created", "response-a") + terminal("response-b"),
		"in progress mismatch":              lifecycle("created", "response-a") + lifecycle("in_progress", "response-b") + terminal("response-a"),
		"queued mismatch":                   lifecycle("queued", "response-a") + terminal("response-b"),
		"created missing object":            frame("response.created", nil) + terminal("response-a"),
		"created missing ID":                frame("response.created", map[string]any{"response": map[string]any{"status": "in_progress"}}) + terminal("response-a"),
		"in progress missing ID":            lifecycle("created", "response-a") + lifecycle("in_progress", nil) + terminal("response-a"),
		"terminal missing ID":               frame("response.completed", map[string]any{"response": map[string]any{"status": "completed", "output": []any{fn("i", "c", "{}")}}}),
		"terminal ID removed after created": lifecycle("created", "response-a") + terminal(nil),
		"empty ID":                          terminal(""), "invalid ID": terminal("bad id"), "oversized ID": terminal(strings.Repeat("x", 257)),
		"event response ID mismatch": lifecycle("created", "response-a") + frame("response.future_metadata", map[string]any{"response_id": "response-b"}) + terminal("response-a"),
		"same event IDs conflict":    frame("response.completed", map[string]any{"response_id": "response-a", "response": map[string]any{"id": "response-b", "status": "completed", "output": []any{fn("i", "c", "{}")}}}),
	} {
		t.Run(name, func(t *testing.T) { assertNoCallsFailure(t, stream) })
	}
	for name, stream := range map[string]string{
		"terminal only":      terminal("response-a"),
		"matching lifecycle": lifecycle("queued", "response-a") + lifecycle("created", "response-a") + lifecycle("in_progress", "response-a") + terminal("response-a"),
		"matching event ID":  frame("response.future_metadata", map[string]any{"response_id": "response-a"}) + terminal("response-a"),
	} {
		t.Run(name, func(t *testing.T) {
			events, _, err := collect(stream)
			if err != nil || len(events) != 1 {
				t.Fatalf("%+v %v", events, err)
			}
		})
	}
}

func TestLifecycleOutputFailsClosed(t *testing.T) {
	for _, kind := range []string{"response.created", "response.in_progress", "response.queued"} {
		t.Run(kind, func(t *testing.T) {
			for name, item := range map[string]map[string]any{
				"reasoning":        {"type": "reasoning", "id": "reason-1", "encrypted_content": "opaque"},
				"hosted execution": {"type": "web_search_call", "id": "search-1"},
				"refusal":          {"type": "message", "id": "refusal-1", "content": []any{map[string]any{"type": "refusal", "refusal": "No"}}},
				"function":         fn("early-item", "early-call", "{}"),
				"message":          msg("early text"),
			} {
				t.Run(name, func(t *testing.T) {
					stream := frame(kind, map[string]any{"response": map[string]any{"id": "response-1", "output": []any{item}}}) + complete(fn("i", "c", "{}"))
					events, _, err := collect(stream)
					if !errors.Is(err, ErrUnsupported) || len(events) != 0 {
						t.Fatalf("lifecycle output escaped: %+v %v", events, err)
					}
				})
			}
			for _, raw := range []string{`null`, `{}`, `"output"`, `1`, `true`, `["output"]`} {
				t.Run("malformed "+raw, func(t *testing.T) {
					assertNoCallsFailure(t, `data: {"type":"`+kind+`","response":{"id":"response-1","output":`+raw+`}}`+"\n\n"+complete(fn("i", "c", "{}")))
				})
			}
			// Both an absent output and an explicitly empty array are valid.
			for _, response := range []map[string]any{{"id": "response-1"}, {"id": "response-1", "output": []any{}}} {
				events, _, err := collect(frame(kind, map[string]any{"response": response}) + complete(fn("i", "c", "{}")))
				if err != nil || len(events) != 1 || events[0].Kind != provider.ToolCalls {
					t.Fatalf("empty lifecycle rejected: %+v %v", events, err)
				}
			}
		})
	}
	events, _, err := collect(complete(fn("i", "c", "{}")))
	if err != nil || len(events) != 1 {
		t.Fatalf("terminal-only completion rejected: %+v %v", events, err)
	}
}

func TestDuplicateJSONMembersFailBeforeEmission(t *testing.T) {
	item := `{"type":"function_call","id":"i","call_id":"c","name":"read_file","arguments":"{}"}`
	response := `{"id":"response-1","status":"completed","output":[` + item + `]}`
	for name, raw := range map[string]string{
		"type":                 `{"type":"response.failed","type":"response.completed","response":` + response + `}`,
		"type alias":           `{"type":"response.failed","TYPE":"response.completed","response":` + response + `}`,
		"status":               `{"type":"response.completed","response":{"id":"response-1","status":"failed","status":"completed","output":[` + item + `]}}`,
		"status Unicode alias": `{"type":"response.completed","response":{"id":"response-1","status":"failed","\u017ftatus":"completed","output":[` + item + `]}}`,
		"output":               `{"type":"response.completed","response":{"id":"response-1","status":"completed","output":[{"type":"reasoning","id":"r"}],"output":[` + item + `]}}`,
		"usage":                `{"type":"response.completed","response":{"id":"response-1","status":"completed","output":[` + item + `],"usage":{"input_tokens":-1},"usage":{"input_tokens":1}}}`,
		"usage token":          `{"type":"response.completed","response":{"id":"response-1","status":"completed","output":[` + item + `],"usage":{"input_tokens":-1,"input_tokens":1}}}`,
		"usage details":        `{"type":"response.completed","response":{"id":"response-1","status":"completed","output":[` + item + `],"usage":{"input_tokens_details":{"cached_tokens":-1,"cached_tokens":0}}}}`,
		"item name":            `{"type":"response.completed","response":{"id":"response-1","status":"completed","output":[{"type":"function_call","id":"i","call_id":"c","name":"hosted_tool","name":"read_file","arguments":"{}"}]}}`,
		"unknown nested":       `{"type":"response.completed","response":` + response + `,"metadata":[{"x":1,"x":2}]}`,
		"escaped key":          `{"type":"response.failed","\u0074ype":"response.completed","response":` + response + `}`,
	} {
		t.Run(name, func(t *testing.T) { assertNoCallsFailure(t, "data: "+raw+"\n\n") })
	}
	assertNoCallsFailure(t, complete(fn("i", "c", `{"file_path":"first","file_path":"second"}`)))
	if _, err := parseUsage([]byte(`{"input_tokens":-1,"input_tokens":1}`)); err == nil {
		t.Fatal("direct usage accepted duplicate")
	}
}

func TestEscapedUnicodeIsNeverRepaired(t *testing.T) {
	for _, escaped := range []string{`\ud800`, `\udfff`, `\ud800x`, `\ud800\u0041`, `\ud800\ud800`} {
		t.Run(escaped, func(t *testing.T) {
			// Literal escapes in envelope values and keys must fail before Go repairs
			// them, even if they are additive metadata that would otherwise be ignored.
			for _, field := range []string{`"metadata":"` + escaped + `"`, `"` + escaped + `":"metadata"`} {
				raw := `{"type":"response.completed",` + field + `,"response":{"id":"response-1","status":"completed","output":[]}}`
				assertNoCallsFailure(t, "data: "+raw+"\n\n")
			}
			// These are escaped again in the event: the completed argument JSON must
			// undergo its own Unicode validation after decoding the outer envelope.
			assertNoCallsFailure(t, complete(fn("i", "c", `{"file_path":"`+escaped+`"}`)))
			assertNoCallsFailure(t, complete(fn("i", "c", `{"`+escaped+`":"value"}`)))
			assertNoCallsFailure(t, added(0, fn("i", "c", ""))+args(0, "i", "delta", `{"file_path":"`+escaped+`"}`)+complete(fn("i", "c", `{"file_path":"`+escaped+`"}`)))
		})
	}
	valid := `{"file_path":"\ud83d\ude42","literal":"\\ud800","n":9007199254740993}`
	events, _, err := collect(complete(fn("i", "c", valid)))
	if err != nil || len(events) != 1 || string(events[0].Calls[0].Input) != valid {
		t.Fatalf("Unicode/precision changed: %+v %v", events, err)
	}
	// An escaped pair may be fragmented across argument-delta events; no
	// executable call is validated until the complete JSON is available.
	events, _, err = collect(added(0, fn("i", "c", "")) + args(0, "i", "delta", `{"file_path":"\ud83d`) + args(0, "i", "delta", `\ude42"}`) + complete(fn("i", "c", `{"file_path":"\ud83d\ude42"}`)))
	if err != nil || len(events) != 1 {
		t.Fatalf("fragmented surrogate pair rejected: %+v %v", events, err)
	}
	raw := `{"type":"response.completed","\ud83d\ude42":"\ud83d\ude42","response":{"id":"response-1","status":"completed","output":[]}}`
	if _, _, err := collect("data: " + raw + "\n\n"); err != nil {
		t.Fatal("valid surrogate pair rejected", err)
	}
}

func TestAnnouncedEmptyPartsAreBoundedAndReconciled(t *testing.T) {
	for _, index := range []int{-1, 128} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			assertNoCallsFailure(t, added(0, emptyMessage("msg-1"))+announcedPart(0, index, "msg-1")+complete(msg(""), fn("i", "c", "{}")))
		})
	}
	for _, index := range []int{0, 1, 127} {
		t.Run(fmt.Sprint("omitted-", index), func(t *testing.T) {
			assertNoCallsFailure(t, added(0, emptyMessage("msg-1"))+announcedPart(0, index, "msg-1")+complete(emptyMessage("msg-1"), fn("i", "c", "{}")))
		})
	}
	assertNoCallsFailure(t, added(0, emptyMessage("msg-1"))+announcedPart(0, 1, "msg-1")+complete(msg(""), fn("i", "c", "{}")))
	// An existing part announcement is idempotent, including after text starts.
	stream := added(0, emptyMessage("msg-1")) + announcedPart(0, 0, "msg-1") + announcedPart(0, 0, "msg-1") + textDelta("hello") + announcedPart(0, 0, "msg-1") + complete(msg("hello"), fn("i", "c", "{}"))
	events, _, err := collect(stream)
	if err != nil || len(events) != 2 || events[0].Text != "hello" || events[1].Kind != provider.ToolCalls {
		t.Fatalf("%+v %v", events, err)
	}
	// Announcements themselves consume the global state budget, not just deltas.
	stream = added(0, emptyMessage("msg-1")) + added(1, emptyMessage("msg-2"))
	for i := 0; i < 128; i++ {
		stream += announcedPart(0, i, "msg-1")
	}
	assertNoCallsFailure(t, stream+announcedPart(1, 0, "msg-2")+complete())
	// Check the error at announcement time, not a later completion limit.
	p := parser{slots: make(map[int]*outputSlot)}
	if _, err := p.item(0, outputItem{Type: "message", ID: "msg-1"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := p.item(1, outputItem{Type: "message", ID: "msg-2"}, false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 128; i++ {
		data := fmt.Sprintf(`{"type":"response.content_part.added","output_index":0,"content_index":%d,"item_id":"msg-1","part":{"type":"output_text","text":""}}`, i)
		if _, err := p.event("", []byte(data)); err != nil {
			t.Fatal(i, err)
		}
	}
	if p.textCount != 128 || len(p.slots[0].texts) != 128 {
		t.Fatal("announced empty parts were not recorded")
	}
	if _, err := p.event("", []byte(`{"type":"response.content_part.added","output_index":1,"content_index":0,"item_id":"msg-2","part":{"type":"output_text","text":""}}`)); err == nil {
		t.Fatal("empty announcement exceeded state budget")
	}
}

func TestStreamJSONWorkBounds(t *testing.T) {
	for name, raw := range map[string]string{
		"depth":  `{"type":"response.future_metadata","extra":` + strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65) + `}`,
		"tokens": `{"type":"response.future_metadata","extra":[` + strings.Repeat("0,", 65536) + `0]}`,
	} {
		t.Run(name, func(t *testing.T) { assertNoCallsFailure(t, "data: "+raw+"\n\n"+complete(fn("i", "c", "{}"))) })
	}
}
