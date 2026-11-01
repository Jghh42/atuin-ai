package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/continuation"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

const generationMarker = "PRIV_SECRET_2817"

type generationValueContext struct {
	context.Context
	Identity credentials.Identity
	Secret   string
}

func generationPresentations[T any](t *testing.T, name string, value T) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		var zero T
		var nilPtr *T
		ptr := &value
		forbidden := []string{generationMarker, hex.EncodeToString([]byte(generationMarker)), base64.StdEncoding.EncodeToString([]byte(generationMarker)), strings.Trim(fmt.Sprint([]byte(generationMarker)), "[]")}
		check := func(s string) {
			t.Helper()
			for _, marker := range forbidden {
				if strings.Contains(s, marker) {
					t.Fatal("private generation presentation exposed payload")
				}
			}
			if strings.Contains(s, "PANIC") || strings.Contains(s, "panicked") {
				t.Fatal("private presentation panicked")
			}
		}
		for _, v := range []any{value, ptr, &ptr, zero, &zero, nilPtr, []T{value, zero}, []*T{ptr, nilPtr}, []any{value, ptr, nilPtr}, map[string]T{"payload": value}, map[string]any{"payload": []any{ptr, nilPtr}}, struct{ Payload any }{value}, &struct{ Payload T }{value}, [1]T{value}} {
			for _, verb := range []string{"%v", "%+v", "%#v", "%w", "%+w", "%#w", "%s", "%q", "%d", "%p", "%#p", "%T", "%x", "%X", "%b", "%f", "%c", "%U", "%020.8w"} {
				check(fmt.Sprintf(verb, v))
				check(fmt.Errorf(verb, v).Error())
			}
			b, e := json.Marshal(v)
			if e != nil {
				t.Fatal("private JSON presentation failed")
			}
			check(string(b))
			for _, text := range []bool{false, true} {
				var out bytes.Buffer
				var h slog.Handler = slog.NewJSONHandler(&out, nil)
				if text {
					h = slog.NewTextHandler(&out, nil)
				}
				slog.New(h).Info("safe", "payload", v)
				check(out.String())
			}
		}
		typ := reflect.TypeFor[T]()
		for j := 0; j < typ.NumField(); j++ {
			if typ.Field(j).IsExported() {
				t.Fatal("generation exposes public payload field")
			}
		}
	})
}
func TestAllNewGenerationPrivatePresentations(t *testing.T) {
	ctx := generationValueContext{Context: context.Background(), Identity: credentials.Identity{Provider: generationMarker, Account: generationMarker}, Secret: generationMarker}
	r := reasoning{reasoningPayload: &reasoningPayload{id: generationMarker, summary: []string{generationMarker}, content: []string{generationMarker}, encrypted: generationMarker, status: generationMarker}}
	generationPresentations(t, "reasoning", r)
	generationPresentations(t, "rawReasoning", rawReasoning{rawReasoningPayload: &rawReasoningPayload{data: []byte(generationMarker)}})
	generationPresentations(t, "reasoningSlot", reasoningSlot{reasoningSlotPayload: &reasoningSlotPayload{id: generationMarker, final: r.digest, done: true}})
	generationPresentations(t, "itemMetadata", itemMetadata{itemMetadataPayload: &itemMetadataPayload{status: generationMarker, phase: generationMarker}})
	generationPresentations(t, "completion", completion{completionPayload: &completionPayload{items: []continuation.Item{continuation.FunctionItem(generationMarker, generationMarker, generationMarker, generationMarker, generationMarker, continuation.Present)}}})
	generationPresentations(t, "replayRun", replayRun{replayRunPayload: &replayRunPayload{items: []json.RawMessage{[]byte(generationMarker)}}})
	generationPresentations(t, "preparedClient", preparedClient{preparedClientPayload: &preparedClientPayload{native: RequestClient{Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return credentials.Credential{}, nil }), URL: generationMarker}}})
	generationPresentations(t, "preparedGeneration", preparedGeneration{preparedGenerationPayload: &preparedGenerationPayload{original: ctx, identity: credentials.Identity{Account: generationMarker}, request: atuin.Request{Messages: []atuin.Message{{Role: "user", Content: []atuin.Block{{Type: "text", Text: generationMarker}, {Type: "tool_use", ID: generationMarker, Input: []byte(generationMarker)}}}}}, instructions: generationMarker, definitionBytes: []byte(generationMarker)}})
	generationPresentations(t, "parser", parser{parserPayload: &parserPayload{responseID: generationMarker, completion: &completion{completionPayload: &completionPayload{items: []continuation.Item{continuation.ReasoningItem(generationMarker, nil, nil, continuation.Omitted, generationMarker, continuation.Present, "", continuation.Omitted)}}}}})
}
