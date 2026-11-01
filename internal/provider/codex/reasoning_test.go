package codex

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

func reasonOutput() map[string]any {
	return map[string]any{"type": "reasoning", "id": "reason-1", "summary": []any{}, "encrypted_content": "PRIVATE-ENCRYPTED"}
}
func reasonPart(kind, text string) map[string]any {
	v := map[string]any{"type": kind, "text": text}
	if kind == "output_text" {
		v["annotations"] = []any{}
	}
	return v
}
func parsedPrepared(input string) ([]provider.Event, *completion, error) {
	var events []provider.Event
	_, c, e := parseConfigured(strings.NewReader(input), func(event provider.Event) error { events = append(events, event); return nil }, true)
	return events, c, e
}
func reasoningAdded(r map[string]any) string {
	return frame("response.output_item.added", map[string]any{"sequence_number": 1, "output_index": 0, "item": r})
}
func assertNoCalls(t *testing.T, events []provider.Event) {
	t.Helper()
	for _, e := range events {
		if e.Kind == provider.ToolCalls {
			t.Fatal("invalid completed output escaped held tool batch")
		}
	}
}

func TestReasoningSchemaRequiredOptionalAndPrivateSink(t *testing.T) {
	for _, mutate := range []func(map[string]any){func(map[string]any) {}, func(r map[string]any) { r["content"] = nil; r["status"] = nil; r["encrypted_content"] = nil }, func(r map[string]any) {
		r["content"] = []any{reasonPart("reasoning_text", "PRIVATE-CONTENT")}
		r["summary"] = []any{reasonPart("summary_text", "PRIVATE-SUMMARY")}
		r["status"] = "completed"
	}} {
		r := reasonOutput()
		mutate(r)
		events, c, e := parsedPrepared(complete(r, msg(" visible \n")))
		if e != nil || c == nil || !c.handled {
			t.Fatal("valid reasoning schema rejected", e)
		}
		var text string
		for _, event := range events {
			text += event.Text
		}
		if text != " visible \n" {
			t.Fatal("private reasoning reached ordinary sink")
		}
		c.close()
	}
	for _, key := range []string{"id", "type", "summary"} {
		for _, bad := range []any{nil, 7, true, "wrong", map[string]any{}} {
			r := reasonOutput()
			r[key] = bad
			if key == "id" && bad == "wrong" {
				continue
			}
			events, _, e := parsedPrepared(complete(fn("f", "c", `{"file_path":"synthetic"}`), r))
			if e == nil {
				t.Fatalf("required field accepted key=%s", key)
			}
			assertNoCalls(t, events)
		}
		r := reasonOutput()
		delete(r, key)
		events, _, e := parsedPrepared(complete(fn("f", "c", `{"file_path":"synthetic"}`), r))
		if e == nil {
			t.Fatal("missing required field accepted")
		}
		assertNoCalls(t, events)
	}
	for _, key := range []string{"content", "encrypted_content", "status"} {
		for _, bad := range []any{7, true, map[string]any{}} {
			r := reasonOutput()
			r[key] = bad
			events, _, e := parsedPrepared(complete(r, fn("f", "c", `{"file_path":"synthetic"}`)))
			if e == nil {
				t.Fatal("wrong optional type accepted")
			}
			assertNoCalls(t, events)
		}
	}
	for _, mutate := range []func(map[string]any){func(r map[string]any) { r["unknown"] = "PRIVATE" }, func(r map[string]any) { r["Summary"] = r["summary"]; delete(r, "summary") }, func(r map[string]any) { r["summary"] = []any{reasonPart("reasoning_text", "PRIVATE")} }, func(r map[string]any) { r["content"] = []any{reasonPart("summary_text", "PRIVATE")} }, func(r map[string]any) { r["summary"] = []any{map[string]any{"type": "summary_text", "text": nil}} }, func(r map[string]any) { r["status"] = "in_progress" }, func(r map[string]any) { r["status"] = "incomplete" }, func(r map[string]any) { r["status"] = "PRIVATE-UNKNOWN-STATUS" }} {
		r := reasonOutput()
		mutate(r)
		events, _, e := parsedPrepared(complete(r, fn("f", "c", `{"file_path":"synthetic"}`)))
		if e == nil {
			t.Fatal("malformed/unknown reasoning accepted")
		}
		assertNoCalls(t, events)
	}
	for _, encrypted := range []any{nil, ""} {
		r := reasonOutput()
		r["encrypted_content"] = encrypted
		_, c, e := parsedPrepared(complete(r, fn("f", "c", `{"file_path":"synthetic"}`)))
		if e != nil || !c.missingState {
			t.Fatal("schema validity confused with local required-state policy", e)
		}
		c.close()
	}
	for _, text := range []string{"", " \t\u0085\u3000\n"} {
		events, _, e := parsedPrepared(complete(reasonOutput(), msg(text)))
		f, ok := provider.FailureOf(e)
		if e == nil || !ok || f.Category() != provider.ReasoningNoVisibleOutput {
			t.Fatal("reasoning-only/whitespace succeeded", e)
		}
		assertNoCalls(t, events)
	}
}
func TestReasoningOpaqueCheckpointsAndFinalOnly(t *testing.T) {
	added := reasonOutput()
	added["encrypted_content"] = "PRIVATE-PROVISIONAL-A"
	added["status"] = "in_progress"
	done := reasonOutput()
	done["encrypted_content"] = "PRIVATE-FINAL-B"
	done["status"] = "completed"
	input := reasoningAdded(added) + frame("response.output_item.done", map[string]any{"sequence_number": 2, "output_index": 0, "item": done}) + complete(done, fn("f", "c", `{"file_path":"synthetic"}`))
	if _, c, e := parsedPrepared(input); e != nil {
		t.Fatal("added opaque incorrectly treated as authoritative", e)
	} else {
		c.close()
	}
	if _, c, e := parsedPrepared(complete(done, fn("f", "c", `{"file_path":"synthetic"}`))); e != nil {
		t.Fatal("final-only checkpoint rejected", e)
	} else {
		c.close()
	}
	for _, mutate := range []func(map[string]any){func(r map[string]any) { r["encrypted_content"] = "PRIVATE-CONFLICT" }, func(r map[string]any) { delete(r, "status") }, func(r map[string]any) { r["content"] = nil }, func(r map[string]any) { r["summary"] = []any{reasonPart("summary_text", "PRIVATE-CONFLICT")} }} {
		conflict := reasonOutput()
		conflict["encrypted_content"] = "PRIVATE-FINAL-B"
		conflict["status"] = "completed"
		mutate(conflict)
		events, _, e := parsedPrepared(reasoningAdded(added) + frame("response.output_item.done", map[string]any{"sequence_number": 2, "output_index": 0, "item": done}) + complete(conflict, fn("f", "c", `{"file_path":"synthetic"}`)))
		if e == nil {
			t.Fatal("authoritative conflict accepted")
		}
		assertNoCalls(t, events)
	}
	// The raw parser must reject ALL reasoning, including terminal text-only.
	for _, wire := range []string{input, complete(done, msg("visible"))} {
		events, _, e := collect(wire)
		if e == nil {
			t.Fatal("raw parser enabled reasoning")
		}
		assertNoCalls(t, events)
	}
}
func TestReasoningFiniteEventMatrix(t *testing.T) {
	for _, kind := range []string{"response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done", "response.reasoning_text.delta", "response.reasoning_text.done"} {
		t.Run(kind, func(t *testing.T) {
			summary := strings.Contains(kind, "summary")
			indexKey := "content_index"
			partKind := "reasoning_text"
			if summary {
				indexKey = "summary_index"
				partKind = "summary_text"
			}
			fields := map[string]any{"sequence_number": 2, "output_index": 0, "item_id": "reason-1", indexKey: 0, "response_id": "response-1"}
			switch {
			case strings.Contains(kind, "_part."):
				fields["part"] = reasonPart(partKind, "PRIVATE-TEXT")
			case strings.HasSuffix(kind, "delta"):
				fields["delta"] = "PRIVATE-TEXT"
			default:
				fields["text"] = "PRIVATE-TEXT"
			}
			final := reasonOutput()
			if summary {
				final["summary"] = []any{reasonPart("summary_text", "PRIVATE-TEXT")}
			} else {
				final["content"] = []any{reasonPart("reasoning_text", "PRIVATE-TEXT")}
			}
			valid := reasoningAdded(reasonOutput()) + frame(kind, fields) + complete(final, fn("f", "c", `{"file_path":"synthetic"}`))
			events, c, e := parsedPrepared(valid)
			if e != nil {
				t.Fatal("allowed event rejected", e)
			}
			c.close()
			for _, event := range events {
				if strings.Contains(event.Text, "PRIVATE") {
					t.Fatal("reasoning event leaked to text")
				}
			}
			for key := range fields {
				if key == "response_id" || key == "type" {
					continue
				}
				for _, value := range []any{nil, -1, true, "PRIVATE-WRONG-TYPE"} {
					copy := map[string]any{}
					for k, v := range fields {
						copy[k] = v
					}
					copy[key] = value
					events, _, e := parsedPrepared(reasoningAdded(reasonOutput()) + frame(kind, copy) + complete(final, fn("f", "c", `{"file_path":"synthetic"}`)))
					if e == nil {
						t.Fatal("wrong required event field accepted", key)
					}
					assertNoCalls(t, events)
				}
				copy := map[string]any{}
				for k, v := range fields {
					copy[k] = v
				}
				delete(copy, key)
				events, _, e := parsedPrepared(reasoningAdded(reasonOutput()) + frame(kind, copy) + complete(final, fn("f", "c", `{"file_path":"synthetic"}`)))
				if e == nil {
					t.Fatal("missing required event field accepted", key)
				}
				assertNoCalls(t, events)
			}
			for _, badType := range []any{nil, 7, "wrong"} {
				copy := map[string]any{}
				for k, v := range fields {
					copy[k] = v
				}
				copy["type"] = badType
				b, _ := json.Marshal(copy)
				events, _, e := parsedPrepared(reasoningAdded(reasonOutput()) + "event: " + kind + "\ndata: " + string(b) + "\n\n" + complete(final, fn("f", "c", `{"file_path":"synthetic"}`)))
				if e == nil {
					t.Fatal("invalid event discriminator accepted")
				}
				assertNoCalls(t, events)
			}
			gap := map[string]any{}
			for k, v := range fields {
				gap[k] = v
			}
			gap["sequence_number"] = 7
			if _, c, e := parsedPrepared(reasoningAdded(reasonOutput()) + frame(kind, gap) + complete(final, fn("f", "c", `{"file_path":"synthetic"}`))); e != nil {
				t.Fatal("valid nonconsecutive sequence rejected", e)
			} else {
				c.close()
			}
			if kind == "response.reasoning_summary_part.done" {
				for _, status := range []any{nil, "incomplete", "completed"} {
					copy := map[string]any{}
					for k, v := range fields {
						copy[k] = v
					}
					copy["status"] = status
					events, c, e := parsedPrepared(reasoningAdded(reasonOutput()) + frame(kind, copy) + complete(final, fn("f", "c", `{"file_path":"synthetic"}`)))
					if status == nil {
						if e != nil {
							t.Fatal("nullable normal part completion rejected", e)
						}
						c.close()
					} else {
						if e == nil {
							t.Fatal("invalid/interrupted part status accepted")
						}
						assertNoCalls(t, events)
					}
				}
			}
			bad := map[string]any{}
			for k, v := range fields {
				bad[k] = v
			}
			bad["PRIVATE-UNKNOWN-FIELD"] = true
			events, _, e = parsedPrepared(reasoningAdded(reasonOutput()) + frame(kind, bad) + complete(final, fn("f", "c", `{"file_path":"synthetic"}`)))
			if e == nil {
				t.Fatal("unknown event field accepted")
			}
			assertNoCalls(t, events)
		})
	}
}
func TestReasoningSharedBoundsSparseAndInterrupted(t *testing.T) {
	for _, count := range []int{128, 129} {
		r := reasonOutput()
		parts := make([]any, count)
		for j := range parts {
			parts[j] = reasonPart("summary_text", "")
		}
		r["summary"] = parts
		events, c, e := parsedPrepared(complete(r, fn("f", "c", `{"file_path":"synthetic"}`)))
		if count == 128 {
			if e != nil {
				t.Fatal("128 shared state control rejected", e)
			}
			c.close()
		} else {
			if e == nil {
				t.Fatal("129 parts accepted")
			}
			assertNoCalls(t, events)
		}
	}
	r := reasonOutput()
	r["summary"] = []any{reasonPart("summary_text", "")}
	sparse := reasoningAdded(reasonOutput()) + frame("response.reasoning_summary_part.added", map[string]any{"sequence_number": 2, "output_index": 0, "item_id": "reason-1", "summary_index": 127, "part": reasonPart("summary_text", "")}) + complete(r, fn("f", "c", `{"file_path":"synthetic"}`))
	events, _, e := parsedPrepared(sparse)
	if e == nil {
		t.Fatal("sparse omitted part accepted")
	}
	assertNoCalls(t, events)
	for _, ending := range []string{frame("response.failed", nil), frame("response.incomplete", nil), frame("response.refusal.delta", nil), frame("response.tool_PRIVATE.delta", nil), ""} {
		events, _, e := parsedPrepared(reasoningAdded(reasonOutput()) + ending)
		if e == nil {
			t.Fatal("interrupted/unknown stream succeeded")
		}
		assertNoCalls(t, events)
	}
	// Shared aggregate ordinary + reasoning, even empty announcements.
	parts := make([]any, 128)
	for j := range parts {
		parts[j] = reasonPart("summary_text", "")
	}
	r["summary"] = parts
	events, _, e = parsedPrepared(complete(r, msg("visible")))
	if e == nil {
		t.Fatal("ordinary and hidden parts did not share128 states")
	}
	assertNoCalls(t, events)
}
func TestReasoningEventBoundariesAndSnapshots(t *testing.T) {
	r := reasonOutput()
	r["summary"] = []any{reasonPart("summary_text", "private é🙂")}
	input := reasoningAdded(reasonOutput()) + frame("response.reasoning_summary_text.delta", map[string]any{"sequence_number": 2, "output_index": 0, "item_id": "reason-1", "summary_index": 0, "delta": "private é🙂"}) + complete(r, msg("visible \n"))
	for split := 0; split < len(input); split++ {
		var text string
		_, c, e := parseConfigured(io.MultiReader(strings.NewReader(input[:split]), strings.NewReader(input[split:])), func(event provider.Event) error { text += event.Text; return nil }, true)
		if e != nil || text != "visible \n" {
			t.Fatal("fragmented event failed", e)
		}
		c.close()
	}
	_, c, e := parseConfigured(&oneByteReader{strings.NewReader(input)}, func(provider.Event) error { return nil }, true)
	if e != nil {
		t.Fatal(e)
	}
	c.close()
	// Invalid Unicode, duplicate/case aliases are rejected globally before decode.
	raw := complete(r, msg("visible"))
	for _, bad := range []string{strings.Replace(raw, `"summary":`, `"Summary":[],"summary":`, 1), strings.Replace(raw, "private é🙂", `\ud800`, 1)} {
		events, _, e := parsedPrepared(bad)
		if e == nil {
			t.Fatal("global JSON gate bypassed")
		}
		assertNoCalls(t, events)
	}
	encoded, _ := json.Marshal(r)
	if bytes.Contains(encoded, []byte("visible")) {
		t.Fatal("bad synthetic fixture")
	}
}
