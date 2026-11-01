package continuation

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

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

const privateMarker = "PRI_SECRET_7791"

type secretValueContext struct {
	context.Context
	Secret string
}

// Exercise values, pointers, typed nils, zero values and nested containers. In
// particular invalid fmt verbs disable ALL method handling recursively in
// Go1.27; successful ordinary Formatter tests alone are insufficient evidence.
func privatePresentations[T any](t *testing.T, name string, value T) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		var zero T
		var nilPointer *T
		pointer := &value
		values := []any{value, pointer, &pointer, zero, &zero, nilPointer, []T{value, zero}, []*T{pointer, nilPointer}, []any{value, pointer, nilPointer}, map[string]T{"payload": value}, map[string]any{"payload": []any{value, pointer, nilPointer}}, struct{ Payload T }{value}, &struct{ Payload any }{pointer}, [1]T{value}}
		forbidden := []string{privateMarker, privateMarker[:8]}
		for _, s := range []string{privateMarker, privateMarker[:8]} {
			b := []byte(s)
			forbidden = append(forbidden, hex.EncodeToString(b), strings.ToUpper(hex.EncodeToString(b)), base64.StdEncoding.EncodeToString(b), base64.RawURLEncoding.EncodeToString(b), strings.Trim(fmt.Sprint(b), "[]"))
		}
		check := func(s string) {
			t.Helper()
			for _, secret := range forbidden {
				if strings.Contains(s, secret) {
					t.Fatal("private presentation exposed raw or encoded payload")
				}
			}
			if strings.Contains(s, "PANIC") || strings.Contains(s, "panicked") {
				t.Fatal("presentation panicked")
			}
		}
		for _, v := range values {
			for _, verb := range []string{"%v", "%+v", "%#v", "%w", "%+w", "%#w", "%s", "%q", "%d", "%p", "%#p", "%T", "%b", "%c", "%o", "%O", "%x", "%X", "%f", "%e", "%U", "%020.8w", "%+020.8v"} {
				check(fmt.Sprintf(verb, v))
				check(fmt.Errorf(verb, v).Error())
			}
			b, e := json.Marshal(v)
			if e != nil {
				t.Fatalf("JSON failed for %T", v)
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
		if typ.Kind() == reflect.Struct {
			for j := 0; j < typ.NumField(); j++ {
				if typ.Field(j).IsExported() {
					t.Fatal("opaque type exposes a mutable/public payload field")
				}
			}
		}
	})
}
func TestEveryOpaquePrivatePresentation(t *testing.T) {
	var raw32 [32]byte
	copy(raw32[:], privateMarker)
	var raw16 [16]byte
	copy(raw16[:], privateMarker)
	var raw8 [8]byte
	copy(raw8[:], privateMarker)
	ctx := secretValueContext{Context: context.Background(), Secret: privateMarker}
	commitment := newCommitment(commitmentPayload{bytes: raw32})
	snapshot := newSnapshotID(snapshotIDPayload{bytes: raw16})
	span := newSpan(spanPayload{bytes: 123, digest: raw32})
	handle := newPublicHandle(publicHandlePayload{value: privateMarker})
	backend := NewBackendCallID(privateMarker)
	part := newPart(partPayload{kind: 2, text: privateMarker})
	recipe := newTextRecipe(textRecipePayload{leading: []byte(privateMarker), spans: []Span{span}, rawDigest: raw32})
	item := newItem(itemPayload{iD: privateMarker, callID: privateMarker, name: privateMarker, arguments: privateMarker, encrypted: privateMarker, status: privateMarker, phase: privateMarker, summary: []Part{part}, content: []Part{part}})
	call := newCall(callPayload{handle: handle, backend: backend, name: privateMarker, publicArguments: commitment})
	prefix := newPrefix(prefixPayload{digest: commitment})
	text := newTextExpectation(textExpectationPayload{digest: commitment})
	segment := newSegment(segmentPayload{snapshot: snapshot, prefix: prefix, text: text, recipe: recipe, calls: []Call{call}, items: []Item{item}})
	record := newRecord(recordPayload{snapshot: snapshot, scope: commitment, segments: []Segment{segment}})
	store := newStore(storePayload{identity: credentials.Identity{Provider: privateMarker, Account: privateMarker}, scope: ctx})
	lease := newLease(leasePayload{store: &store, original: ctx, scope: ctx, linked: ctx})
	key := newKeys(keysPayload{root: raw32, id: raw8})
	privatePresentations(t, "PublicHandle", handle)
	privatePresentations(t, "BackendCallID", backend)
	privatePresentations(t, "Commitment", commitment)
	privatePresentations(t, "SnapshotID", snapshot)
	privatePresentations(t, "ScopeInput", NewScopeInput(credentials.Identity{Provider: privateMarker, Account: privateMarker}, privateMarker, privateMarker, commitment))
	privatePresentations(t, "Prefix", prefix)
	privatePresentations(t, "TextExpectation", text)
	privatePresentations(t, "Span", span)
	privatePresentations(t, "TextRecipe", recipe)
	privatePresentations(t, "Captured", Captured{capturedPayload: &capturedPayload{recipe: recipe, clientDigest: raw32, clientBytes: 7791, meaningful: true}})
	privatePresentations(t, "Part", part)
	privatePresentations(t, "Item", item)
	privatePresentations(t, "Call", call)
	privatePresentations(t, "Segment", segment)
	privatePresentations(t, "Record", record)
	privatePresentations(t, "Match", newMatch(matchPayload{text: privateMarker, results: []int{7791}}))
	privatePresentations(t, "keys", key)
	privatePresentations(t, "handleParts", newHandleParts(handlePartsPayload{key: raw8, snapshot: snapshot, mac: raw16}))
	privatePresentations(t, "Store", store)
	privatePresentations(t, "Lease", lease)
	privatePresentations(t, "leaseContext", newLeaseContext(leaseContextPayload{context: ctx, l: &lease}))
	privatePresentations(t, "emitContext", newEmitContext(emitContextPayload{context: ctx, publication: ctx, source: ctx, scope: ctx}))
	privatePresentations(t, "TextCapture", newTextCapture(textCapturePayload{leading: []byte(privateMarker)}))
	privatePresentations(t, "encoder", newEncoder(encoderPayload{b: []byte(privateMarker)}))
	privatePresentations(t, "decoder", newDecoder(decoderPayload{b: []byte(privateMarker)}))
	privatePresentations(t, "Error", newError(errorPayload{code: 255}))
}
func TestOpaqueConstructorsDeepCopyMutableOwnership(t *testing.T) {
	leading := []byte(" \t")
	spans := []Span{newSpan(spanPayload{bytes: 2})}
	recipe := newTextRecipe(textRecipePayload{leading: leading, spans: spans})
	leading[0] = 'x'
	spans[0].bytes = 99
	if string(recipe.leading) != " \t" || recipe.spans[0].bytes != 2 {
		t.Fatal("recipe borrowed caller buffers")
	}
	parts := []Part{newPart(partPayload{text: "original"})}
	item := newItem(itemPayload{summary: parts})
	parts[0].text = "changed"
	if item.summary[0].text != "original" {
		t.Fatal("item borrowed nested part")
	}
	calls := []Call{newCall(callPayload{name: "original", backend: NewBackendCallID("backend")})}
	items := []Item{item}
	segment := newSegment(segmentPayload{calls: calls, items: items, recipe: recipe})
	calls[0].name = "changed"
	items[0].summary[0].text = "changed"
	recipe.leading[0] = 'z'
	if segment.calls[0].name != "original" || segment.items[0].summary[0].text != "original" || string(segment.recipe.leading) != " \t" {
		t.Fatal("segment borrowed nested ownership")
	}
	segments := []Segment{segment}
	record := newRecord(recordPayload{segments: segments})
	segments[0].calls[0].backend.value = "changed"
	segment.recipe.leading[0] = 'y'
	if record.segments[0].calls[0].backend.value != "backend" || string(record.segments[0].recipe.leading) != " \t" {
		t.Fatal("record borrowed ancestral mutable state")
	}
	cloned := cloneRecord(record)
	record.Clear()
	if cloned.segments[0].calls[0].name != "original" || string(cloned.segments[0].recipe.leading) != " \t" {
		t.Fatal("clearing one record destroyed independent snapshot")
	}
}
