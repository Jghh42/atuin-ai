package integration

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Jghh42/atuin-ai/tests/compatfixture"
	"github.com/Jghh42/atuin-ai/tests/testutil"
)

func TestSyntheticCompatibilityCorpus(t *testing.T) {
	cases, e := compatfixture.Load("../../testdata/compat/phase8.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			n := testutil.New(t, []testutil.Turn{c.Turn}, true)
			status, body, header := testutil.PostWithSession(t, n.URL, c.Request)
			if status != 200 {
				t.Fatalf("status %d", status)
			}
			events, e := compatfixture.Events(body)
			if e != nil {
				t.Fatal(e)
			}
			if e = compatfixture.ValidateSession(events, header, compatfixture.RequestedSession(c.Request)); e != nil {
				t.Fatal(e)
			}
			want := []string{"status", "status"}
			if c.Turn.Text != "" {
				want = append(want, "text")
			}
			if c.Difference == "mixed-rejection" {
				want = append(want, "error")
			} else {
				for range c.Turn.Calls {
					want = append(want, "tool_call")
				}
				if len(c.Turn.Calls) > 0 && c.Turn.Calls[0].Name != "suggest_command" {
					want = append(want, "status")
				}
				want = append(want, "done")
			}
			got := []string{}
			for _, v := range events {
				got = append(got, v.Event)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("event sequence %v want %v", got, want)
			}
			if events[0].Data["state"] != "processing" || events[1].Data["state"] != "thinking" {
				t.Fatal("initial lifecycle changed")
			}
			for _, event := range events {
				if event.Event == "tool_call" {
					var found bool
					for _, call := range c.Turn.Calls {
						if call.ID == event.Data["id"] {
							found = true
							if event.Data["name"] != call.Name {
								t.Fatal("tool identity changed")
							}
							var input map[string]any
							_ = json.Unmarshal(call.Input, &input)
							actual, ok := event.Data["input"].(map[string]any)
							if !ok {
								t.Fatal("tool arguments not object")
							}
							if c.Difference == "enhanced-risk" {
								if actual["danger"] != "high" {
									t.Fatal("risk backstop not enforced")
								}
								delete(input, "danger")
								delete(actual, "danger")
								delete(actual, "danger_notes")
							}
							if !reflect.DeepEqual(input, actual) {
								t.Fatalf("tool arguments changed: %v / %v", actual, input)
							}
						}
					}
					if !found {
						t.Fatal("unknown call emitted")
					}
				}
			}
			if c.Turn.UnknownUsage {
				u, ok := events[len(events)-1].Data["usage"].(map[string]any)
				if !ok {
					t.Fatal("missing usage")
				}
				for _, key := range []string{"input_tokens", "output_tokens", "total_tokens", "cached_tokens", "cache_creation_tokens", "input_cost", "output_cost", "total_cost", "provider_cost"} {
					_, exists := u[key]
					if !exists || u[key] != nil {
						t.Fatal("unknown usage/cost fabricated or omitted")
					}
				}
			}
			client, up, models, exchanges, rotations := n.Captures()
			if len(client) != 1 || len(up) != 1 || models != 1 || exchanges != 1 || rotations != 0 {
				t.Fatal("unexpected request/generation/lifecycle counts")
			}
		})
	}
}

func TestNormalizationDoesNotHideBehavior(t *testing.T) {
	a := []compatfixture.Event{{Event: "tool_call", Data: map[string]any{"id": "call-a", "input": map[string]any{"danger": "low"}}}}
	b := []compatfixture.Event{{Event: "tool_call", Data: map[string]any{"id": "call-a", "input": map[string]any{"danger": "high"}}}}
	if reflect.DeepEqual(compatfixture.Normalize(a), compatfixture.Normalize(b)) {
		t.Fatal("normalization hid risk")
	}
	if reflect.DeepEqual(compatfixture.Normalize(a), compatfixture.Normalize(append(b, compatfixture.Event{Event: "done"}))) {
		t.Fatal("normalization hid terminal behavior")
	}
}
