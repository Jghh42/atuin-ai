package codex

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

func schemaMessage(text string) map[string]any {
	return map[string]any{"id": "schema-message", "type": "message", "role": "assistant", "status": "completed", "phase": "commentary", "content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}}}
}
func checkpointRetainedBytes(v reflect.Value) int {
	if !v.IsValid() {
		return 0
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return 0
		}
		return int(v.Type().Size()) + checkpointRetainedBytes(v.Elem())
	case reflect.Struct:
		n := 0
		for j := 0; j < v.NumField(); j++ {
			n += checkpointRetainedBytes(v.Field(j))
		}
		return n
	case reflect.Slice, reflect.Array:
		n := 0
		for j := 0; j < v.Len(); j++ {
			n += checkpointRetainedBytes(v.Index(j))
		}
		return n
	case reflect.String:
		return int(v.Type().Size()) + len(v.String())
	default:
		return int(v.Type().Size())
	}
}
func TestReviewReasoningDoneRetainsOnlyBoundedCheckpoints(t *testing.T) {
	p := parser{parserPayload: &parserPayload{prepared: true, slots: map[int]*outputSlot{}, sink: func(provider.Event) error { return nil }}}
	for j := 0; j < 128; j++ {
		r := reasonOutput()
		r["id"] = fmt.Sprintf("reason-%d", j)
		r["encrypted_content"] = "ENC-" + strings.Repeat("z", 900<<10)
		b, _ := json.Marshal(r)
		if _, e := p.item(j, outputItem{Type: "reasoning", rawReason: &rawReasoning{rawReasoningPayload: &rawReasoningPayload{data: b}}}, true); e != nil {
			t.Fatal("bounded final checkpoint rejected")
		}
	}
	retained := 0
	for _, s := range p.slots {
		retained += checkpointRetainedBytes(reflect.ValueOf(s.reason))
	}
	if len(p.slots) != 128 || p.textCount != 0 {
		t.Fatal("checkpoint/part accounting changed")
	}
	r := reasonOutput()
	r["id"] = "extra"
	b, _ := json.Marshal(r)
	if _, e := p.item(128, outputItem{Type: "reasoning", rawReason: &rawReasoning{rawReasoningPayload: &rawReasoningPayload{data: b}}}, true); e == nil || len(p.slots) != 128 {
		t.Fatal("checkpoint slot bound bypassed")
	}
	t.Logf("128 completed checkpoints retain %d owned scalar/digest bytes, not 128 raw payloads", retained)
	if retained > 128*512 {
		t.Fatalf("DONE checkpoints retained %d payload bytes", retained)
	}
}
func TestReviewReasoningSnapshotIndexBeforeArithmetic(t *testing.T) {
	for _, index := range []int64{-9223372036854775808, -1, 128, 9223372036854775807, 4611686018427387904} {
		r := reasonOutput()
		r["summary"] = []any{reasonPart("summary_text", "")}
		wire := reasoningAdded(r) + frame("response.reasoning_summary_part.added", map[string]any{"sequence_number": 2, "item_id": "reason-1", "output_index": 0, "summary_index": index, "part": reasonPart("summary_text", "")}) + complete(r, fn("f", "backend", `{"file_path":"owned"}`))
		events, _, e := parsedPrepared(wire)
		if e == nil {
			t.Fatalf("index %d bypassed bound", index)
		}
		assertNoCalls(t, events)
	}
}
func TestReviewExecutionMetadataStickyRejects(t *testing.T) {
	for _, field := range []string{"caller", "async"} {
		for _, where := range []string{"added", "done", "final"} {
			f := fn("f", "backend", `{"file_path":"owned"}`)
			if field == "caller" {
				f[field] = map[string]any{"type": "program", "caller_id": "program-private"}
			} else {
				f[field] = true
			}
			wire := ""
			if where != "final" {
				wire = frame("response.output_item."+where, map[string]any{"output_index": 0, "item": f})
				f = fn("f", "backend", `{"file_path":"owned"}`)
			}
			wire += complete(f, reasonOutput())
			for _, prepared := range []bool{true, false} {
				var events []provider.Event
				_, _, e := parseConfigured(strings.NewReader(wire), func(event provider.Event) error { events = append(events, event); return nil }, prepared)
				if e == nil || len(events) > 0 {
					t.Fatalf("%s/%s execution metadata escaped", field, where)
				}
			}
		}
	}
}
func TestReviewOrdinaryDoneMetadataCannotBeErased(t *testing.T) {
	for _, kind := range []string{"message", "function"} {
		for _, field := range []string{"status", "phase"} {
			if kind == "function" && field == "phase" {
				continue
			}
			item := schemaMessage("visible")
			if kind == "function" {
				item = fn("f", "backend", `{"file_path":"owned"}`)
				item["status"] = "completed"
			}
			snapshot := map[string]any{}
			for k, v := range item {
				snapshot[k] = v
			}
			if field == "status" {
				item[field] = "incomplete"
			} else {
				item[field] = "final_answer"
			}
			wire := frame("response.output_item.done", map[string]any{"output_index": 0, "item": item}) + complete(snapshot, reasonOutput(), fn("other", "other-backend", `{"file_path":"owned"}`))
			events, _, e := parsedPrepared(wire)
			if e == nil {
				t.Fatalf("%s %s DONE conflict lost", kind, field)
			}
			assertNoCalls(t, events)
		}
	}
}
func TestReviewReplayMessageRequiredMetadata(t *testing.T) {
	f := newPreparedFixture(t, func(f *preparedFixture, body map[string]any) string {
		if f.posts == 1 {
			return complete(reasonOutput(), schemaMessage("visible "), fn("f", "backend", `{"file_path":"owned"}`))
		}
		items := body["input"].([]any)
		found := false
		for _, row := range items {
			v := row.(map[string]any)
			if v["id"] == "schema-message" {
				found = true
				if v["status"] != "completed" || v["phase"] != "commentary" {
					t.Error("required message status/phase lost")
				}
				part := v["content"].([]any)[0].(map[string]any)
				a, ok := part["annotations"].([]any)
				if !ok || len(a) != 0 {
					t.Error("required empty annotations lost")
				}
			}
		}
		if !found {
			t.Error("message missing")
		}
		return complete(schemaMessage("terminal"))
	})
	r := initialRequest()
	first := runPrepared(t, f.client, r)
	if len(first.calls) != 1 {
		t.Fatal("valid schema batch rejected")
	}
	r = appendPublicHistory(r, first, false, false)
	_ = runPrepared(t, f.client, r)
}
func TestReviewNonemptyAnnotationsRejectProtectedPublication(t *testing.T) {
	f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
		m := schemaMessage("visible")
		m["content"].([]any)[0].(map[string]any)["annotations"] = []any{map[string]any{"type": "file_path", "file_id": "PRIVATE", "index": 0}}
		return complete(reasonOutput(), m, fn("f", "backend", `{"file_path":"owned"}`))
	})
	result := runPrepared(t, f.client, initialRequest())
	if len(result.calls) != 0 || len(result.failures) == 0 {
		t.Fatal("unsupported annotations silently discarded")
	}
}
