package compatfixture

import (
	"reflect"
	"testing"
)

func text(s string, extra bool) Event {
	d := map[string]any{"content": s}
	if extra {
		d["unexpected"] = true
	}
	return Event{Event: "text", Data: d}
}
func TestTextNormalizationPreservesNoncontentFieldsAndOrder(t *testing.T) {
	plain := []Event{text("ab", false)}
	if !reflect.DeepEqual(Normalize([]Event{text("a", false), text("b", false)}), plain) {
		t.Fatal("content-only segmentation must normalize")
	}
	for _, events := range [][]Event{
		{text("a", false), text("b", true)},
		{text("a", true), text("b", false)},
		{text("a", true), text("b", true)},
		{text("a", false), text("b", true), text("c", false)},
	} {
		got := Normalize(events)
		if !reflect.DeepEqual(got, events) {
			t.Fatalf("metadata/order lost: %#v", got)
		}
		if reflect.DeepEqual(got, plain) {
			t.Fatal("unexpected field disappeared")
		}
		if _, exists := got[0].Data["content"]; !exists {
			t.Fatal("content lost")
		}
	}
	a, b := Normalize([]Event{text("a", true), text("b", false)}), Normalize([]Event{text("b", false), text("a", true)})
	if reflect.DeepEqual(a, b) {
		t.Fatal("chunk ordering disappeared")
	}
	nonstring := []Event{{Event: "text", Data: map[string]any{"content": 1}}, text("b", false)}
	if !reflect.DeepEqual(Normalize(nonstring), nonstring) {
		t.Fatal("nonstring chunk merged")
	}
}
