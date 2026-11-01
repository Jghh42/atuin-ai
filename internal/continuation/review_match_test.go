package continuation

import (
	"testing"

	"github.com/Jghh42/atuin-ai/internal/atuin"
)

// These invoke the library directly, intentionally bypassing the public
// normalizer: MAC authenticity/membership is NOT occurrence authorization.
func TestProtectedOccurrencesCannotEscapeMatchedSegments(t *testing.T) {
	k := testKeys()
	prefix := []atuin.Message{textMessage("user", "original")}
	r := sampleRecord(t, k, 2, "", prefix, 1)
	s := r.segments[0]
	valid := history(s, prefix, "", false, true)
	exchange := []atuin.Message{{Role: "assistant", Content: []atuin.Block{callBlock(s, 0)}}, resultMessage(s, 0)}
	for _, tc := range []struct {
		name  string
		extra []atuin.Message
	}{
		{"duplicate-result-suffix", []atuin.Message{resultMessage(s, 0)}},
		{"duplicate-exchange-suffix", exchange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := append(append([]atuin.Message(nil), valid...), tc.extra...)
			if _, e := k.verifyRecord(r, messages, r.scope); e == nil {
				t.Fatal("authentic protected occurrence accepted outside verified segment")
			}
		})
	}
	// A later public unprotected exchange remains a legitimate suffix.
	suffix := append(append([]atuin.Message(nil), valid...), textMessage("user", "next"), atuin.Message{Role: "assistant", Content: []atuin.Block{{Type: "tool_use", ID: "legacy-unprotected", Name: "read_file", Input: []byte(`{"file_path":"synthetic"}`)}}}, atuin.Message{Role: "user", Content: []atuin.Block{{Type: "tool_result", ToolUseID: "legacy-unprotected", Content: "synthetic"}}})
	if _, e := k.verifyRecord(r, suffix, r.scope); e != nil {
		t.Fatal("valid unprotected suffix rejected", e)
	}

	for _, where := range []string{"prefix", "gap", "suffix"} {
		for _, kind := range []string{"result", "exchange"} {
			t.Run(where+"-old-"+kind, func(t *testing.T) {
				first := sampleRecord(t, k, 2, "", prefix, 1)
				older := first.segments[0]
				extra := []atuin.Message{resultMessage(older, 0)}
				if kind == "exchange" {
					extra = []atuin.Message{{Role: "assistant", Content: []atuin.Block{callBlock(older, 0)}}, resultMessage(older, 0)}
				}
				messages := prefix
				if where == "prefix" {
					// Bind this adversarial prefix deliberately. Verification must reject its
					// occurrences independently of a matching digest, not trust the normalizer.
					messages = append(append([]atuin.Message(nil), prefix...), extra...)
					p, e := k.prefix(messages)
					if e != nil {
						t.Fatal(e)
					}
					first.segments[0].prefix = p
					older = first.segments[0]
				}
				messages = history(older, messages, "", true, false)
				messages = append(messages, textMessage("user", "between protected segments"))
				if where == "gap" {
					messages = append(messages, extra...)
				}
				second := sampleRecord(t, k, 1, "", messages, 2)
				second.segments = append(first.segments, second.segments...)
				second.expires = first.expires
				messages = history(second.segments[1], messages, "", false, false)
				if where == "suffix" {
					messages = append(messages, extra...)
				}
				if _, e := k.verifyRecord(second, messages, second.scope); e == nil {
					t.Fatal("authentic ancestor occurrence outside matched interval accepted")
				}
			})
		}
	}
}
