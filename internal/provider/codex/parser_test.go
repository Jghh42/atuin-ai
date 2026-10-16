package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/provider"
)

func frame(kind string, fields map[string]any) string {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["type"] = kind
	b, _ := json.Marshal(fields)
	return "event: " + kind + "\ndata: " + string(b) + "\n\n"
}
func complete(items ...map[string]any) string {
	if items == nil {
		items = []map[string]any{}
	}
	return frame("response.completed", map[string]any{"response": map[string]any{"id": "response-1", "status": "completed", "output": items}})
}
func fn(id, call, args string) map[string]any {
	return map[string]any{"type": "function_call", "id": id, "call_id": call, "name": "read_file", "arguments": args}
}
func msg(text string) map[string]any {
	return map[string]any{"type": "message", "id": "msg-1", "content": []any{map[string]any{"type": "output_text", "text": text}}}
}
func added(index int, item map[string]any) string {
	return frame("response.output_item.added", map[string]any{"output_index": index, "item": item})
}
func args(index int, id, kind, value string) string {
	field := "delta"
	if kind == "done" {
		field = "arguments"
	}
	return frame("response.function_call_arguments."+kind, map[string]any{"output_index": index, "item_id": id, field: value})
}
func textDelta(text string) string {
	return frame("response.output_text.delta", map[string]any{"output_index": 0, "content_index": 0, "item_id": "msg-1", "delta": text})
}
func collect(input string) ([]provider.Event, provider.Usage, error) {
	var events []provider.Event
	u, e := parseStream(strings.NewReader(input), func(e provider.Event) error { events = append(events, e); return nil })
	return events, u, e
}

func TestSSEEveryBoundary(t *testing.T) {
	input := ": heartbeat\n\n" + added(0, msg("")) + added(1, fn("item-u", "call-u", "")) + textDelta("hé🙂") + args(1, "item-u", "delta", `{"file_path":"é🙂`) + args(1, "item-u", "done", `{"file_path":"é🙂","n":9007199254740993}`) + complete(msg("hé🙂"), fn("item-u", "call-u", `{"file_path":"é🙂","n":9007199254740993}`))
	for _, newline := range []string{"\n", "\r\n"} {
		stream := strings.ReplaceAll(input, "\n", newline)
		for split := 0; split <= len(stream); split++ {
			var got string
			_, err := parseStream(io.MultiReader(strings.NewReader(stream[:split]), strings.NewReader(stream[split:])), func(e provider.Event) error { got += e.Text; return nil })
			if err != nil || got != "hé🙂" {
				t.Fatalf("split %d: %q %v", split, got, err)
			}
		}
		_, err := parseStream(&oneByteReader{strings.NewReader(stream)}, func(provider.Event) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
	}
	// Multiline data is joined with LF, not concatenated or parsed per line.
	_, _, err := collect("event: response.completed\ndata: {\ndata: \"type\":\"response.completed\",\ndata: \"response\":{\"id\":\"response-1\",\"status\":\"completed\",\"output\":[]}}\n\n")
	if err != nil {
		t.Fatal(err)
	}
}

type oneByteReader struct{ io.Reader }

func (r *oneByteReader) Read(b []byte) (int, error) {
	if len(b) > 1 {
		b = b[:1]
	}
	return r.Reader.Read(b)
}

func TestParallelCallsSnapshotsAndDuplicates(t *testing.T) {
	a, b := fn("item-a", "call-a", ""), fn("item-b", "call-b", "")
	stream := added(0, a) + added(1, b) + added(0, a) + args(1, "item-b", "delta", `{"file_`) + args(0, "item-a", "delta", `{"file_path":"a"`) + args(1, "item-b", "done", `{"file_path":"b"}`) + args(0, "item-a", "done", `{"file_path":"a"}`)
	stream += args(0, "item-a", "done", `{"file_path":"a"}`) + frame("response.output_item.done", map[string]any{"output_index": 0, "item": fn("item-a", "call-a", `{"file_path":"a"}`)})
	events, _, err := collect(stream + complete(fn("item-a", "call-a", `{"file_path":"a"}`), fn("item-b", "call-b", `{"file_path":"b"}`)))
	if err != nil || len(events) != 1 || len(events[0].Calls) != 2 {
		t.Fatalf("%+v %v", events, err)
	}
	if events[0].Calls[0].ID != "call-a" || events[0].Calls[1].ID != "call-b" || string(events[0].Calls[0].Input) != `{"file_path":"a"}` {
		t.Fatalf("%+v", events)
	}
	for _, tail := range []string{"", frame("response.failed", nil), frame("response.incomplete", nil), "data: [DONE]\n\n"} {
		events, _, err := collect(stream + tail)
		if err == nil || len(events) != 0 {
			t.Fatalf("calls escaped failed response: %v %v", events, err)
		}
	}
}
func TestSequencePolicy(t *testing.T) {
	delta := func(seq int, text string) string {
		return frame("response.output_text.delta", map[string]any{"sequence_number": seq, "output_index": 0, "content_index": 0, "item_id": "msg-1", "delta": text})
	}
	events, _, err := collect(added(0, msg("")) + delta(3, "a") + delta(3, "a") + delta(4, "a") + complete(msg("aa")))
	if err != nil || len(events) != 2 {
		t.Fatalf("%v %v", events, err)
	}
	for _, bad := range []string{delta(3, "b"), delta(2, "a")} {
		if _, _, err := collect(added(0, msg("")) + delta(3, "a") + bad + complete(msg("a"))); err == nil {
			t.Fatal("conflicting sequence accepted")
		}
	}
}
func TestParserFailurePolicies(t *testing.T) {
	cases := map[string]string{
		"empty": "", "arbitrary 200": "not SSE", "DONE": "data: [DONE]\n\n", "bad JSON": "data: {\n\n",
		"truncated": strings.TrimSuffix(complete(), "\n"), "event mismatch": strings.Replace(complete(), "event: response.completed", "event: other", 1),
		"failed": frame("response.failed", nil), "incomplete": frame("response.incomplete", nil), "missing response": frame("response.completed", nil),
		"wrong status":      frame("response.completed", map[string]any{"response": map[string]any{"status": "in_progress", "output": []any{}}}),
		"refusal":           frame("response.refusal.delta", map[string]any{"delta": "No"}),
		"refusal snapshot":  complete(map[string]any{"type": "message", "id": "m", "content": []any{map[string]any{"type": "refusal", "refusal": "No"}}}),
		"reasoning":         complete(map[string]any{"type": "reasoning", "id": "r", "encrypted_content": "opaque"}),
		"reasoning channel": frame("response.reasoning_text.delta", nil) + complete(),
		"hosted tool":       complete(map[string]any{"type": "web_search_call", "id": "web"}),
		"custom tool":       complete(map[string]any{"type": "custom_tool_call", "id": "c", "call_id": "c"}),
		"bad id":            complete(fn("item", "bad id", "{}")), "bad name": complete(map[string]any{"type": "function_call", "id": "a", "call_id": "c", "name": "a.b", "arguments": "{}"}),
		"bad args": complete(fn("i", "c", "[1]")), "bad JSON args": complete(fn("i", "c", "{bad}")),
		"duplicate call id": complete(fn("i", "c", "{}"), fn("j", "c", "{}")), "duplicate item id": complete(fn("i", "c", "{}"), fn("i", "d", "{}")),
		"missing index":      frame("response.output_item.added", map[string]any{"item": fn("a", "c", "")}),
		"wrong item id":      added(0, fn("a", "c", "")) + args(0, "c", "delta", "{}") + complete(fn("a", "c", "{}")),
		"conflicting args":   added(0, fn("a", "c", "")) + args(0, "a", "delta", "{\"x\":1}") + complete(fn("a", "c", "{\"x\":2}")),
		"delta after done":   added(0, fn("a", "c", "")) + args(0, "a", "done", "{}") + args(0, "a", "delta", " ") + complete(fn("a", "c", "{}")),
		"missing final item": added(0, fn("a", "c", "")) + complete(),
		"conflicting text":   added(0, msg("")) + textDelta("hello") + complete(msg("bye")),
		"invalid UTF8":       "data: {\"type\":\"\xff\"}\n\n",
		"namespace":          complete(map[string]any{"type": "function_call", "id": "i", "call_id": "c", "name": "read_file", "namespace": "hosted", "arguments": "{}"}),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			events, _, err := collect(input)
			if err == nil {
				t.Fatal("accepted malformed stream")
			}
			for _, e := range events {
				if e.Kind == provider.ToolCalls {
					t.Fatal("tools emitted")
				}
			}
			if strings.Contains(err.Error(), "opaque") {
				t.Fatal("unsafe error")
			}
		})
	}
	if _, _, err := collect(frame("response.future_metadata", map[string]any{"hello": "future"}) + complete()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := collect(complete(msg("snapshot only"))); err != nil {
		t.Fatal(err)
	}
}
func TestParserBounds(t *testing.T) {
	// Exact wire budget includes ':', LF and the blank delimiter.
	if _, _, err := collect(":" + strings.Repeat("x", config.MaxProviderEventBytes-3) + "\n\n" + complete()); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{":", "data: ", "ignored: "} {
		if _, _, err := collect(prefix + strings.Repeat("x", config.MaxProviderEventBytes) + "\n\n" + complete()); err == nil {
			t.Fatal("oversized frame accepted")
		}
	}
	if _, _, err := collect(strings.Repeat(": heartbeat\n", 100000) + "\n" + complete()); err == nil {
		t.Fatal("comment budget bypass")
	}
	long := strings.Repeat("x", 70000)
	if _, _, err := collect(": " + long + "\n\n" + complete()); err != nil {
		t.Fatal("default Scanner bound", err)
	}
	a := `{"x":"` + strings.Repeat("x", config.MaxCallArgumentsBytes) + `"}`
	if _, _, err := collect(complete(fn("a", "c", a))); err == nil {
		t.Fatal("oversized args accepted")
	}
	stream := added(0, fn("a", "c", ""))
	for i := 0; i < 5; i++ {
		stream += args(0, "a", "delta", strings.Repeat("x", 65536))
	}
	if _, _, err := collect(stream + complete()); err == nil {
		t.Fatal("delta args bound")
	}
	var items []map[string]any
	for i := 0; i < 33; i++ {
		items = append(items, fn(fmt.Sprint("i", i), fmt.Sprint("c", i), "{}"))
	}
	if _, _, err := collect(complete(items...)); err == nil {
		t.Fatal("call count bound")
	}
	// Many small heartbeat frames are allowed; only per-event work is buffered.
	if _, _, err := collect(strings.Repeat(": heartbeat\n\n", 100000) + complete()); err != nil {
		t.Fatal(err)
	}
}
func TestUsage(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"input_tokens":0,"output_tokens":0,"input_tokens_details":{"cached_tokens":0}}`, `{"input_tokens":12,"output_tokens":3,"total_tokens":15,"input_tokens_details":{"cached_tokens":4}}`} {
		u, err := parseUsage([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, "12") && (u.InputTokens != 12 || u.OutputTokens != 3 || u.TotalTokens != 15 || u.CachedTokens != 4 || !u.CachedKnown || !u.TotalKnown) {
			t.Fatal(u)
		}
		if raw == `{}` && (u.InputKnown || u.TotalKnown) {
			t.Fatal(u)
		}
	}
	for _, raw := range []string{`{"input_tokens":-1}`, `{"output_tokens":1.2}`, `{"total_tokens":9223372036854775808}`, `{"input_tokens":1,"output_tokens":2,"total_tokens":4}`, `{"input_tokens":1,"input_tokens_details":{"cached_tokens":2}}`, fmt.Sprintf(`{"input_tokens":%d,"output_tokens":1}`, int64(math.MaxInt64))} {
		if _, err := parseUsage([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	stream := frame("response.completed", map[string]any{"response": map[string]any{"id": "response-1", "status": "completed", "output": []any{}, "usage": map[string]any{"input_tokens": 12, "output_tokens": 3, "input_tokens_details": map[string]int{"cached_tokens": 4}}}})
	_, u, err := collect(stream)
	if err != nil || u.TotalTokens != 15 || u.CachedTokens != 4 {
		t.Fatalf("%+v %v", u, err)
	}
}
func TestContentPartsAndFinalReconciliation(t *testing.T) {
	part := func(kind, text string) string {
		return frame("response.content_part."+kind, map[string]any{"output_index": 0, "content_index": 0, "item_id": "msg-1", "part": map[string]any{"type": "output_text", "text": text}})
	}
	stream := added(0, msg("")) + part("added", "") + textDelta("hello") + part("done", "hello") + part("done", "hello") + frame("response.output_text.done", map[string]any{"output_index": 0, "content_index": 0, "item_id": "msg-1", "text": "hello"})
	events, _, err := collect(stream + complete(msg("hello")))
	if err != nil || len(events) != 1 {
		t.Fatalf("%v %v", events, err)
	}
	if _, _, err := collect(stream + part("done", "different") + complete(msg("hello"))); err == nil {
		t.Fatal("conflicting part accepted")
	}
	// Invalid completion accounting must prevent even otherwise complete tools.
	events, _, err = collect(frame("response.completed", map[string]any{"response": map[string]any{"id": "response-1", "status": "completed", "output": []any{fn("i", "c", "{}")}, "usage": map[string]int{"input_tokens": -1}}}))
	if err == nil || len(events) != 0 {
		t.Fatalf("%v %v", events, err)
	}
}

func TestParserSinkFailure(t *testing.T) {
	sentinel := errors.New("sink failed")
	for _, stream := range []string{added(0, msg("")) + textDelta("x") + complete(msg("x")), complete(fn("i", "c", "{}"))} {
		_, err := parseStream(strings.NewReader(stream), func(provider.Event) error { return sentinel })
		if !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
	}
}
func TestSyntheticFixture(t *testing.T) {
	b, err := os.ReadFile("../../../testdata/responses/phase5-text.sse")
	if err != nil {
		t.Fatal(err)
	}
	events, _, err := collect(string(b))
	if err != nil || len(events) != 1 || events[0].Text != "Hello 🙂" {
		t.Fatalf("%v %v", events, err)
	}
}
func FuzzResponsesSSE(f *testing.F) {
	f.Add([]byte(complete()))
	f.Add([]byte(added(0, msg("")) + textDelta("hé🙂") + complete(msg("hé🙂"))))
	f.Add([]byte(complete(fn("i", "c", "{}"))))
	f.Add([]byte("data: [DONE]\n\n"))
	f.Add([]byte(added(1, fn("a", "call-a", "")) + complete(fn("b", "call-b", "{}"))))
	f.Add([]byte(frame("response.web_search_call.in_progress", nil) + complete()))
	f.Add([]byte(`data: {"type":"response.failed","type":"response.completed","response":{"id":"r","status":"completed","output":[]}}` + "\n\n"))
	f.Add([]byte(complete(fn("i", "c", `{"file_path":"\ud800"}`))))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 2*config.MaxProviderEventBytes {
			return
		}
		batches := 0
		_, _ = parseStream(bytes.NewReader(b), func(e provider.Event) error {
			if e.Kind == provider.ToolCalls {
				batches++
				if len(e.Calls) > 32 {
					t.Fatal("too many calls")
				}
			}
			return nil
		})
		if batches > 1 {
			t.Fatal("duplicate batch")
		}
	})
}
