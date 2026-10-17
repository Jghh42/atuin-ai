package differential

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/Jghh42/atuin-ai/tests/compatfixture"
)

func usageExpected(native, unknown bool) map[string]any {
	u := map[string]any{}
	for _, key := range []string{"input_tokens", "output_tokens", "total_tokens", "cached_tokens", "cache_creation_tokens"} {
		if native {
			u[key] = nil
		} else {
			u[key] = float64(0)
		}
	}
	if !unknown {
		u["input_tokens"], u["output_tokens"], u["total_tokens"] = float64(10), float64(5), float64(15)
	}
	for _, key := range []string{"input_cost", "output_cost", "total_cost", "provider_cost"} {
		if native {
			u[key] = nil
		} else {
			u[key] = "nil"
		}
	}
	return u
}
func expectedPolicy(c compatfixture.Case, native bool, session string) []compatfixture.Event {
	events := []compatfixture.Event{{Event: "status", Data: map[string]any{"state": "processing"}}, {Event: "status", Data: map[string]any{"state": "thinking"}}}
	if c.Difference == "mixed-rejection" && native {
		return append(events, compatfixture.Event{Event: "error", Data: map[string]any{"message": "LLM request failed, please retry", "code": "upstream_error"}})
	}
	if c.Turn.Text != "" {
		events = append(events, compatfixture.Event{Event: "text", Data: map[string]any{"content": c.Turn.Text}})
	}
	for _, call := range c.Turn.Calls {
		var input map[string]any
		_ = json.Unmarshal(call.Input, &input)
		if native && c.Difference == "enhanced-risk" {
			input["danger"] = "high"
			input["danger_notes"] = "Privileged execution can change protected system state."
		}
		events = append(events, compatfixture.Event{Event: "tool_call", Data: map[string]any{"id": call.ID, "name": call.Name, "input": input}})
	}
	return append(events, compatfixture.Event{Event: "done", Data: map[string]any{"session_id": session, "usage": usageExpected(native, c.Turn.UnknownUsage)}})
}
func compareCase(c compatfixture.Case, native []compatfixture.Event, header string, reference []compatfixture.Event, refHeader string) error {
	requested := compatfixture.RequestedSession(c.Request)
	if e := compatfixture.ValidateSession(native, header, requested); e != nil {
		return fmt.Errorf("native session: %w", e)
	}
	if e := compatfixture.ValidateSession(reference, refHeader, requested); e != nil {
		return fmt.Errorf("reference session: %w", e)
	}
	// Usage normalization is permitted only after checking all actual raw token /
	// cost fields. Unknown native counts are null; the original codec emits "nil"
	// cost strings. No missing keys or unrelated additions disappear unnoticed.
	for _, side := range []struct {
		events []compatfixture.Event
		native bool
	}{{native, true}, {reference, false}} {
		for _, event := range side.events {
			if event.Event == "done" && !reflect.DeepEqual(event.Data["usage"], usageExpected(side.native, c.Turn.UnknownUsage)) {
				return fmt.Errorf("raw usage contract changed")
			}
		}
	}
	if c.Difference != "" {
		switch c.Difference {
		case "mixed-rejection", "enhanced-risk", "unknown-usage":
		default:
			return fmt.Errorf("unasserted difference")
		}
		// COMPLETE expected events/payloads, including exact generic error and all
		// reference calls/order/status/done fields. Only declared risk/notes differ.
		if !reflect.DeepEqual(compatfixture.Normalize(native), compatfixture.Normalize(expectedPolicy(c, true, header))) {
			return fmt.Errorf("native intentional-policy payload changed")
		}
		if !reflect.DeepEqual(compatfixture.Normalize(reference), compatfixture.Normalize(expectedPolicy(c, false, refHeader))) {
			return fmt.Errorf("reference intentional-policy payload changed")
		}
		if c.Difference == "enhanced-risk" {
			// Explicitly reconcile ONLY the already-asserted risk/notes delta, then
			// compare every remaining event and payload unchanged (UUID/usage allowed).
			b, _ := json.Marshal(reference)
			var adjusted []compatfixture.Event
			_ = json.Unmarshal(b, &adjusted)
			input := adjusted[2].Data["input"].(map[string]any)
			input["danger"] = "high"
			input["danger_notes"] = "Privileged execution can change protected system state."
			if !reflect.DeepEqual(compatfixture.Normalize(native), compatfixture.Normalize(adjusted)) {
				return fmt.Errorf("enhanced-risk hid unrelated event/payload delta")
			}
		}
		return nil
	}
	if !reflect.DeepEqual(compatfixture.Normalize(native), compatfixture.Normalize(reference)) {
		return fmt.Errorf("positive event/payload sequence differs")
	}
	return nil
}
func cloneEvents(in []compatfixture.Event) []compatfixture.Event {
	b, _ := json.Marshal(in)
	var out []compatfixture.Event
	_ = json.Unmarshal(b, &out)
	return out
}
func TestPositiveAndPolicyTextMetadataCannotNormalizeAway(t *testing.T) {
	const a = "01900000-0000-7000-8000-000000000001"
	const b = "01900000-0000-7000-8000-000000000002"
	for _, difference := range []string{"", "unknown-usage"} {
		c := compatfixture.Case{Difference: difference, Request: json.RawMessage(`{}`)}
		c.Turn.Text = "ab"
		n, r := expectedPolicy(c, true, a), expectedPolicy(c, false, b)
		if err := compareCase(c, n, a, r, b); err != nil {
			t.Fatal(err)
		}
		for _, nativeSide := range []bool{true, false} {
			for _, extraFirst := range []bool{false, true} {
				nn, rr := cloneEvents(n), cloneEvents(r)
				parts := []compatfixture.Event{{Event: "text", Data: map[string]any{"content": "a"}}, {Event: "text", Data: map[string]any{"content": "b"}}}
				idx := 1
				if extraFirst {
					idx = 0
				}
				parts[idx].Data["unexpected"] = true
				split := func(v []compatfixture.Event) []compatfixture.Event {
					out := append([]compatfixture.Event{}, v[:2]...)
					out = append(out, parts...)
					return append(out, v[3:]...)
				}
				if nativeSide {
					nn = split(nn)
				} else {
					rr = split(rr)
				}
				if compareCase(c, nn, a, rr, b) == nil {
					t.Fatalf("%q policy/positive metadata disappeared (native=%v first=%v)", difference, nativeSide, extraFirst)
				}
			}
		}
	}
}

func TestIntentionalPoliciesRejectUnrelatedMutations(t *testing.T) {
	cases, e := compatfixture.Load("../../testdata/compat/phase8.json")
	if e != nil {
		t.Fatal(e)
	}
	const a = "01900000-0000-7000-8000-000000000001"
	const b = "01900000-0000-7000-8000-000000000002"
	for _, c := range cases {
		if c.Difference == "" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			native, reference := expectedPolicy(c, true, a), expectedPolicy(c, false, b)
			if e := compareCase(c, native, a, reference, b); e != nil {
				t.Fatal(e)
			}
			mutate := func(label string, nativeSide bool, change func([]compatfixture.Event)) {
				t.Run(label, func(t *testing.T) {
					n, r := cloneEvents(native), cloneEvents(reference)
					if nativeSide {
						change(n)
					} else {
						change(r)
					}
					if compareCase(c, n, a, r, b) == nil {
						t.Fatal("unrelated policy mutation normalized away")
					}
				})
			}
			mutate("native-status", true, func(v []compatfixture.Event) { v[0].Data["state"] = "waiting_for_tools" })
			mutate("reference-status-extra", false, func(v []compatfixture.Event) { v[1].Data["unrelated"] = true })
			mutate("reference-done-extra", false, func(v []compatfixture.Event) { v[len(v)-1].Data["credits"] = 1.0 })
			mutate("reference-cost", false, func(v []compatfixture.Event) { v[len(v)-1].Data["usage"].(map[string]any)["total_cost"] = 0.0 })
			if c.Difference == "mixed-rejection" {
				mutate("native-error-message", true, func(v []compatfixture.Event) { v[2].Data["message"] = "unrelated error" })
				mutate("native-error-code", true, func(v []compatfixture.Event) { v[2].Data["code"] = "wrong" })
				mutate("reference-call-order", false, func(v []compatfixture.Event) { v[2], v[3] = v[3], v[2] })
			}
			if len(c.Turn.Calls) > 0 {
				mutate("reference-call-id", false, func(v []compatfixture.Event) { v[2].Data["id"] = "changed" })
				mutate("reference-call-name", false, func(v []compatfixture.Event) { v[2].Data["name"] = "write_file" })
				mutate("reference-call-input", false, func(v []compatfixture.Event) { v[2].Data["input"].(map[string]any)["command"] = "different" })
			}
		})
	}
}
