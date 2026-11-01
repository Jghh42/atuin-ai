package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

// Complete reachable field/payload accounting: no outputItem field, metadata
// pointer, raw slice, map entry or hidden-part hash state is excluded. Allocator
// bookkeeping is bounded separately by the128slot/shared128part count ceilings.
func slotOwnedBytes(v reflect.Value, seen map[uintptr]bool) int {
	if !v.IsValid() {
		return 0
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return int(v.Type().Size())
		}
		ptr := v.Pointer()
		if seen[ptr] {
			return int(v.Type().Size())
		}
		seen[ptr] = true
		return int(v.Type().Size()) + slotOwnedBytes(v.Elem(), seen)
	case reflect.Interface:
		if v.IsNil() {
			return int(v.Type().Size())
		}
		return int(v.Type().Size()) + slotOwnedBytes(v.Elem(), seen)
	case reflect.String:
		return int(v.Type().Size()) + len(v.String())
	case reflect.Struct:
		n := 0
		for j := 0; j < v.NumField(); j++ {
			n += slotOwnedBytes(v.Field(j), seen)
		}
		return n
	case reflect.Slice, reflect.Array:
		n := 0
		if v.Kind() == reflect.Slice {
			n = int(v.Type().Size())
		}
		for j := 0; j < v.Len(); j++ {
			n += slotOwnedBytes(v.Index(j), seen)
		}
		return n
	case reflect.Map:
		n := int(v.Type().Size())
		iter := v.MapRange()
		for iter.Next() {
			n += slotOwnedBytes(iter.Key(), seen) + slotOwnedBytes(iter.Value(), seen)
		}
		return n
	default:
		return int(v.Type().Size())
	}
}
func TestReviewOrdinarySlotOwnedBytesAllInapplicableFields(t *testing.T) {
	for _, field := range []string{"name", "call_id", "arguments", "composite"} {
		for _, event := range []string{"added", "done"} {
			t.Run(field+"/"+event, func(t *testing.T) {
				p := parser{parserPayload: &parserPayload{prepared: true, slots: map[int]*outputSlot{}, sink: func(provider.Event) error { return nil }}}
				for index := 0; index < 128; index++ {
					m := schemaMessage("")
					m["id"] = fmt.Sprintf("owned-%d", index)
					m["content"] = []any{}
					switch field {
					case "composite":
						m["name"] = strings.Repeat("n", 300<<10)
						m["call_id"] = strings.Repeat("c", 300<<10)
						m["arguments"] = strings.Repeat("a", 256<<10)
					case "arguments":
						m[field] = strings.Repeat("a", 256<<10)
					default:
						m[field] = strings.Repeat("x", 900<<10)
					}
					wire := frame("response.output_item."+event, map[string]any{"output_index": index, "item": m})
					if len(wire) > 1<<20 {
						t.Fatal("adversarial fixture exceeds unchanged frame cap")
					}
					name := "response.output_item." + event
					data := strings.TrimSuffix(strings.SplitN(wire, "data: ", 2)[1], "\n\n")
					if _, e := p.event(name, []byte(data)); e != nil {
						t.Fatal("bounded ordinary legacy checkpoint rejected", e)
					}
				}
				for _, s := range p.slots {
					if s.item.Name != "" || s.item.CallID != "" || s.item.Arguments != "" || s.args != "" || s.item.Content != nil || s.item.Namespace != nil || s.item.metadata != nil || s.item.rawReason != nil || len(s.item.ID) > 256 {
						t.Fatal("nonminimal ordinary checkpoint retained inapplicable storage")
					}
				}
				n := slotOwnedBytes(reflect.ValueOf(p.slots), map[uintptr]bool{})
				t.Logf("128 %s/%s checkpoints: %d owned bytes", field, event, n)
				if n > 128*2048 {
					t.Fatal("ordinary checkpoint retained inapplicable payloads")
				}
				if len(p.slots) != 128 || p.calls != 0 || p.textCount != 0 {
					t.Fatal("shared checkpoint limits changed")
				}
			})
		}
	}
}
func TestReviewRequiredDonePresenceCannotBeErased(t *testing.T) {
	for _, where := range []string{"item-content", "item-text", "part-text"} {
		for _, value := range []any{"missing", nil, 7, "wrong-array-type"} {
			t.Run(fmt.Sprintf("%s/%v", where, value), func(t *testing.T) {
				f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
					done := schemaMessage("")
					done["content"] = []any{}
					final := schemaMessage("")
					final["content"] = []any{}
					wire := ""
					if where == "item-content" {
						if value == "missing" {
							delete(done, "content")
						} else {
							done["content"] = value
						}
					} else {
						part := map[string]any{"type": "output_text", "annotations": []any{}}
						if value != "missing" {
							part["text"] = value
						}
						final["content"] = []any{map[string]any{"type": "output_text", "text": "", "annotations": []any{}}}
						if where == "part-text" {
							added := schemaMessage("")
							added["content"] = []any{}
							wire = frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + frame("response.content_part.done", map[string]any{"output_index": 0, "item_id": "schema-message", "content_index": 0, "part": part})
						} else {
							done["content"] = []any{part}
						}
					}
					if where != "part-text" {
						wire = frame("response.output_item.done", map[string]any{"output_index": 0, "item": done})
					}
					return wire + complete(final, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
				})
				result := runPrepared(t, f.client, initialRequest())
				if len(result.calls) != 0 || len(result.failures) != 1 || f.posts != 1 {
					t.Fatal("invalid required DONE presence released tools or retried")
				}
				if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
					t.Fatal("invalid DONE committed private root key")
				}
			})
		}
	}
}
func TestReviewStickyFailuresNeverResetByMissingRole(t *testing.T) {
	for _, field := range []string{"status", "phase", "annotations", "caller", "async"} {
		t.Run(field, func(t *testing.T) {
			f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
				added := schemaMessage("")
				delete(added, "role")
				added["content"] = []any{}
				final := schemaMessage("")
				final["content"] = []any{}
				if field == "caller" || field == "async" {
					added = fn("f", "backend", `{"file_path":"owned"}`)
					if field == "caller" {
						added[field] = map[string]any{"type": "program", "caller_id": "private"}
					} else {
						added[field] = true
					}
					return frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + complete(fn("f", "backend", `{"file_path":"owned"}`), reasonOutput())
				}
				switch field {
				case "status":
					added[field] = "unsupported"
				case "phase":
					added[field] = "unsupported"
				case "annotations":
					added["content"] = []any{map[string]any{"type": "output_text", "text": "", "annotations": nil}}
					final["content"] = []any{map[string]any{"type": "output_text", "text": "", "annotations": []any{}}}
				}
				return frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + complete(final, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
			})
			result := runPrepared(t, f.client, initialRequest())
			if len(result.calls) != 0 || len(result.failures) != 1 {
				t.Fatal("supplied invalid metadata bit reset")
			}
			if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
				t.Fatal("invalid provisional metadata created private key")
			}
		})
	}
}
func TestReviewRequiredPresenceEmptyAndProvisionalControls(t *testing.T) {
	for _, parts := range []bool{false, true} {
		f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string {
			added := schemaMessage("")
			delete(added, "role")
			added["status"] = "in_progress"
			added["content"] = []any{}
			done := schemaMessage("")
			done["content"] = []any{}
			if parts {
				done["content"] = []any{map[string]any{"type": "output_text", "text": "", "annotations": []any{}}}
			}
			return frame("response.output_item.added", map[string]any{"output_index": 0, "item": added}) + frame("response.output_item.done", map[string]any{"output_index": 0, "item": done}) + complete(done, reasonOutput(), fn("f", "backend", `{"file_path":"owned"}`))
		})
		result := runPrepared(t, f.client, initialRequest())
		if len(result.calls) != 1 || len(result.failures) != 0 {
			t.Fatal("valid present-empty or provisional missing-role transition rejected")
		}
	}
}
func TestReviewDigestDistinguishesExcludedPayloadShape(t *testing.T) {
	for _, key := range []string{"text", "content"} {
		var baseline [32]byte
		for index, value := range []any{"missing", nil, "", []any{}, 7, true, map[string]any{}} {
			raw := map[string]json.RawMessage{}
			if value != "missing" {
				raw[key], _ = json.Marshal(value)
			}
			var digest [32]byte
			if key == "text" {
				digest = textMetadata(raw).digest
			} else {
				digest = outputMetadata(raw, "message").digest
			}
			if index == 0 {
				baseline = digest
			} else if baseline == digest {
				t.Fatal("excluded payload presence/type digest alias")
			}
		}
	}
}
