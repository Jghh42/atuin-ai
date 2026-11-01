package codex

import "testing"

func TestReviewPhaseAndTextPartPresenceReplay(t *testing.T) {
	for _, phase := range []any{"omitted", nil, "commentary", "final_answer"} {
		for _, logprobs := range []any{"omitted", nil, []any{}} {
			f := newPreparedFixture(t, func(f *preparedFixture, body map[string]any) string {
				if f.posts == 1 {
					m := schemaMessage("visible ")
					if phase == "omitted" {
						delete(m, "phase")
					} else {
						m["phase"] = phase
					}
					part := m["content"].([]any)[0].(map[string]any)
					if logprobs != "omitted" {
						part["logprobs"] = logprobs
					}
					return complete(reasonOutput(), m, fn("f", "backend", `{"file_path":"owned"}`))
				}
				for _, row := range body["input"].([]any) {
					m := row.(map[string]any)
					if m["id"] != "schema-message" {
						continue
					}
					value, exists := m["phase"]
					if phase == "omitted" {
						if exists {
							t.Error("omitted phase invented")
						}
					} else if !exists || value != phase {
						t.Error("message phase presence/value lost")
					}
					if m["role"] != "assistant" || m["status"] != "completed" {
						t.Error("required message status/role invalid")
					}
					part := m["content"].([]any)[0].(map[string]any)
					if a, ok := part["annotations"].([]any); !ok || len(a) != 0 {
						t.Error("required annotations absent/invalid")
					}
					lp, present := part["logprobs"]
					if _, want := logprobs.([]any); want {
						if v, ok := lp.([]any); !present || !ok || len(v) != 0 {
							t.Error("empty logprobs lost")
						}
					} else if present {
						t.Error("omitted/null output logprobs must omit input field")
					}
				}
				return complete(schemaMessage("terminal"))
			})
			r := initialRequest()
			first := runPrepared(t, f.client, r)
			if len(first.calls) != 1 {
				t.Fatal("supported schema/presence rejected")
			}
			runPrepared(t, f.client, appendPublicHistory(r, first, false, false))
		}
	}
}
