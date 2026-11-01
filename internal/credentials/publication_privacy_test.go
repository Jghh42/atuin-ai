package credentials

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

type privateValueContext struct {
	context.Context
	Identity Identity
	Secret   string
}

func checkOpaquePresentation[T any](t *testing.T, value T) {
	t.Helper()
	var zero T
	var nilPointer *T
	ptr := &value
	for _, v := range []any{value, ptr, &ptr, zero, &zero, nilPointer, []T{value, zero}, []*T{ptr, nilPointer}, []any{value, ptr, nilPointer}, map[string]any{"payload": value}, struct{ Payload any }{ptr}, &struct{ Payload any }{value}} {
		check := func(s string) {
			t.Helper()
			for _, secret := range []string{"PRI_SECRET_7791", hex.EncodeToString([]byte("PRI_SECRET_7791")), strings.Trim(fmt.Sprint([]byte("PRI_SECRET_7791")), "[]")} {
				if strings.Contains(s, secret) {
					t.Fatal("new private object leaked context/identity/payload")
				}
			}
			if strings.Contains(s, "PANIC") || strings.Contains(s, "panicked") {
				t.Fatal("presentation panicked")
			}
		}
		for _, verb := range []string{"%v", "%+v", "%#v", "%w", "%+w", "%#w", "%s", "%q", "%d", "%p", "%#p", "%T", "%x", "%X", "%b", "%f", "%U"} {
			check(fmt.Sprintf(verb, v))
			check(fmt.Errorf(verb, v).Error())
		}
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal("JSON presentation failed")
		}
		check(string(b))
		for _, text := range []bool{true, false} {
			var out bytes.Buffer
			var handler slog.Handler = slog.NewJSONHandler(&out, nil)
			if text {
				handler = slog.NewTextHandler(&out, nil)
			}
			slog.New(handler).Info("safe", "value", v)
			check(out.String())
		}
	}
}
func TestNewPublicationObjectsHideValueContextsAndIdentity(t *testing.T) {
	ctx := privateValueContext{Context: context.Background(), Identity: Identity{Provider: "PRI_SECRET_7791", Account: "PRI_SECRET_7791"}, Secret: "PRI_SECRET_7791"}
	state := publicationState{payload: &publicationStatePayload{original: ctx, life: ctx, linked: ctx}}
	lease := PublicationLease{state: &state}
	bound := newPublicationContext(&state)
	checkOpaquePresentation(t, state)
	checkOpaquePresentation(t, lease)
	checkOpaquePresentation(t, bound)
	checkOpaquePresentation(t, ContinuationEntry{payload: &continuationEntryPayload{name: "PRI_SECRET_7791", bytes: 7791}})
	checkOpaquePresentation(t, ContinuationDirectory{})
}
