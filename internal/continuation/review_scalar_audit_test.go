package continuation

import (
	"strings"
	"testing"
)

// Audit every variable-width scalar family and narrowed integer wrapper, not
// just the two originally reported examples. Oversized records never need to
// allocate ciphertext, and all errors retain fixed opaque presentations.
func TestAllCodecScalarCeilingsRejectBeforeWrite(t *testing.T) {
	k := testKeys()
	for _, tc := range []struct {
		name   string
		mutate func(Record)
	}{
		{"call-name", func(r Record) { r.segments[0].calls[0].name = strings.Repeat("n", 257) }},
		{"item-name", func(r Record) { r.segments[0].items[2].name = strings.Repeat("n", 257) }},
		{"backend-ID", func(r Record) { r.segments[0].calls[0].backend = NewBackendCallID(strings.Repeat("i", (2<<20)+1)) }},
		{"item-ID", func(r Record) { r.segments[0].items[0].iD = strings.Repeat("i", (2<<20)+1) }},
		{"item-call-ID", func(r Record) { r.segments[0].items[2].callID = strings.Repeat("i", (2<<20)+1) }},
		{"public-handle", func(r Record) {
			r.segments[0].calls[0].handle = newPublicHandle(publicHandlePayload{value: strings.Repeat("h", 91)})
		}},
		{"arguments", func(r Record) { r.segments[0].items[2].arguments = `{"x":"` + strings.Repeat("a", (256<<10)-7) + `"}` }},
		{"encrypted", func(r Record) { r.segments[0].items[0].encrypted = strings.Repeat("e", (2<<20)+1) }},
		{"part-text", func(r Record) { r.segments[0].items[0].summary[0].text = strings.Repeat("s", (2<<20)+1) }},
		{"status", func(r Record) {
			r.segments[0].items[0].statusPresence = 2
			r.segments[0].items[0].status = strings.Repeat("s", 33)
		}},
		{"leading", func(r Record) { r.segments[0].recipe.leading = []byte(strings.Repeat(" ", (1<<20)+1)) }},
		{"raw-bytes", func(r Record) { r.segments[0].recipe.rawBytes = (5 << 20) + 1 }},
		{"span-bytes", func(r Record) { r.segments[0].recipe.spans[0].bytes = (5 << 20) + 1 }},
		{"part-reference", func(r Record) { r.segments[0].items[1].content[0].span = 4097 }},
		{"prefix-uint32", func(r Record) { r.segments[0].prefix.messages = ^uint32(0) }},
		{"visible-uint32", func(r Record) { r.segments[0].text.bytes = ^uint32(0) }},
		{"item-kind-uint8", func(r Record) { r.segments[0].items[0].kind = 255 }},
		{"part-kind-uint8", func(r Record) { r.segments[0].items[0].summary[0].kind = 255 }},
		{"content-presence", func(r Record) { r.segments[0].items[0].contentPresence = 3 }},
		{"encrypted-presence", func(r Record) { r.segments[0].items[0].encryptedPresence = 3 }},
		{"status-presence", func(r Record) { r.segments[0].items[0].statusPresence = 3 }},
		{"invalid-UTF8-name", func(r Record) { setFunctionName(r, "\xff") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := scalarRecord(t, k)
			tc.mutate(r)
			if e := validateRecord(r); e == nil {
				t.Fatal("scalar escaped shared validation")
			}
			if b, e := encodeRecord(r); e == nil {
				clear(b)
				t.Fatal("scalar escaped binary writer")
			}
			if b, e := k.seal(r); e == nil {
				clear(b)
				t.Fatal("scalar escaped AEAD writer")
			}
		})
	}
}
