package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

func TestReviewMinimalSlotsPreserveRawLegacyIdentity(t *testing.T) {
	for _, field := range []string{"name", "call_id", "arguments"} {
		m := schemaMessage("")
		m["content"] = []any{}
		m["name"] = "legacy-name"
		m["call_id"] = "legacy-call"
		m["arguments"] = "unused-original"
		wire := frame("response.output_item.added", map[string]any{"output_index": 0, "item": m})
		wire += frame("response.output_item.added", map[string]any{"output_index": 0, "item": m})
		final := map[string]any{}
		for k, v := range m {
			final[k] = v
		}
		final[field] = "changed-unused"
		_, e := parseStream(strings.NewReader(wire+complete(final)), func(provider.Event) error { return nil })
		if field == "arguments" {
			if e != nil {
				t.Fatal("raw legacy unused argument acceptance tightened")
			}
		} else if e == nil {
			t.Fatal("raw legacy name/call_id identity reconciliation lost")
		}
	}
	m := schemaMessage("")
	m["content"] = []any{}
	m["call_id"] = "same"
	wire := frame("response.output_item.added", map[string]any{"output_index": 0, "item": m}) + complete(m, fn("function", "same", `{"file_path":"owned"}`))
	if _, e := parseStream(strings.NewReader(wire), func(provider.Event) error { return nil }); e == nil {
		t.Fatal("raw legacy cross-kind call_id duplicate comparison lost")
	}
}
func TestReviewMetadataBitsMonotonicAcrossFieldsAndMapOrders(t *testing.T) {
	for _, kind := range []string{"message", "function_call"} {
		for _, bad := range []string{"status", "phase", "annotations", "caller", "async"} {
			if kind == "message" && (bad == "caller" || bad == "async") || kind == "function_call" && (bad == "phase" || bad == "annotations") {
				continue
			}
			for _, reverse := range []bool{false, true} {
				raw := map[string]json.RawMessage{}
				keys := []string{"type", "id", "status", "content"}
				if reverse {
					keys = []string{"content", "status", "id", "type"}
				}
				for _, key := range keys {
					switch key {
					case "type":
						raw[key], _ = json.Marshal(kind)
					case "id":
						raw[key] = json.RawMessage(`"id"`)
					case "status":
						raw[key] = json.RawMessage(`"completed"`)
					case "content":
						if kind == "message" {
							raw[key] = json.RawMessage(`[]`)
						}
					}
				}
				switch bad {
				case "status":
					raw[bad] = json.RawMessage(`"unsupported"`)
				case "phase":
					raw[bad] = json.RawMessage(`"unsupported"`)
				case "annotations":
					raw["content"] = json.RawMessage(`[{"type":"output_text","text":"","annotations":null}]`)
				case "caller":
					raw[bad] = json.RawMessage(`{"type":"program","caller_id":"private"}`)
				case "async":
					raw[bad] = json.RawMessage(`true`)
				}
				m := outputMetadata(raw, kind)
				if bad == "caller" || bad == "async" {
					if !m.executionInvalid {
						t.Fatal("supplied invalid execution flag cleared")
					}
				} else if !m.sticky || !m.unsupported {
					t.Fatal("supplied invalid metadata flags cleared by omitted later fields")
				}
			}
		}
	}
}
func TestReviewAuthoritativeMissingMetadataFailureAccumulates(t *testing.T) {
	for _, part := range []bool{false, true} {
		p := parser{parserPayload: &parserPayload{prepared: true, slots: map[int]*outputSlot{}, sink: func(provider.Event) error { return nil }}}
		m := schemaMessage("")
		if part {
			m["content"] = []any{map[string]any{"type": "output_text", "annotations": []any{}}}
		} else {
			delete(m, "content")
		}
		raw, _ := json.Marshal(m)
		var item outputItem
		if json.Unmarshal(raw, &item) != nil {
			t.Fatal("fixture decode")
		}
		s, e := p.item(0, item, true)
		if e != nil {
			t.Fatal("missing metadata should remain a bounded invalid checkpoint")
		}
		if !s.projectionUnsupported {
			t.Fatal("ALL authoritative unsupported metadata must accumulate, including omitted fields")
		}
	}
}
