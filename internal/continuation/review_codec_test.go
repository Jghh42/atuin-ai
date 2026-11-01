package continuation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func setFunctionName(r Record, name string) {
	for _, s := range r.segments {
		for j := range s.calls {
			s.calls[j].name = name
		}
		for j := range s.items {
			if s.items[j].kind == 2 {
				s.items[j].name = name
			}
		}
	}
}

// Ordinary visible text is referenced, not archived. This creates coherent raw
// and public layouts without relying on the production parser or its frame cap.
func setTextLayout(k *keys, r Record, client, leading string) {
	s := r.segments[len(r.segments)-1]
	raw := leading + client
	// The regression must be able to construct deliberately invalid metadata,
	// independently of the now-bounded public text capture helper.
	if uint64(len(client)) > uint64(^uint32(0)) || uint64(len(raw)) > uint64(^uint32(0)) {
		panic("synthetic fixture count overflow")
	}
	s.text = k.textDigest(uint32(len(client)), sha256.Sum256([]byte(client)))
	s.recipe = newTextRecipe(textRecipePayload{leading: []byte(leading), rawBytes: uint32(len(raw)), rawDigest: sha256.Sum256([]byte(raw)), spans: []Span{newSpan(spanPayload{bytes: uint32(len(raw)), digest: sha256.Sum256([]byte(raw))})}})
	for j := range s.items {
		if s.items[j].kind == 1 {
			s.items[j].content = []Part{newPart(partPayload{kind: 1, span: 0})}
		}
	}
}
func scalarRecord(t testing.TB, k *keys) Record {
	return sampleRecord(t, k, 1, "x", []atuin.Message{textMessage("user", "synthetic")}, 1)
}

func TestCodecRejectsUnreplayableScalarBeforeSeal(t *testing.T) {
	k := testKeys()
	for _, tc := range []struct {
		name   string
		mutate func(Record)
	}{
		{"name-257", func(r Record) { setFunctionName(r, strings.Repeat("n", 257)) }},
		{"visible-4MiB-plus1", func(r Record) { setTextLayout(k, r, strings.Repeat("v", (4<<20)+1), "") }},
		{"prefix-count-overflow", func(r Record) { r.segments[0].prefix.messages = 65537 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := scalarRecord(t, k)
			tc.mutate(r)
			b, e := encodeRecord(r)
			if e == nil {
				decoded, de := decodeRecord(b)
				decoded.Clear()
				clear(b)
				t.Errorf("over-ceiling record encoded; decode accepted=%v", de == nil)
			}
			sealed, e := k.seal(r)
			if e == nil {
				opened, oe := k.open(sealed)
				opened.Clear()
				clear(sealed)
				t.Errorf("over-ceiling record sealed; AEAD reopen accepted=%v", oe == nil)
			}
		})
	}
}
func TestCodecScalarBoundaryRoundTrips(t *testing.T) {
	k := testKeys()
	for _, tc := range []struct {
		name   string
		mutate func(Record)
	}{
		{"name-256", func(r Record) { setFunctionName(r, strings.Repeat("n", 256)) }},
		{"name-256-UTF8-bytes", func(r Record) { setFunctionName(r, strings.Repeat("é", 128)) }},
		{"visible-4MiB-raw-5MiB", func(r Record) { setTextLayout(k, r, strings.Repeat("v", 4<<20), strings.Repeat(" ", 1<<20)) }},
		{"prefix-count-65536", func(r Record) { r.segments[0].prefix.messages = 65536 }},
		{"arguments-256KiB", func(r Record) {
			for _, i := range r.segments[0].items {
				if i.kind == 2 {
					i.arguments = `{"x":"` + strings.Repeat("a", (256<<10)-8) + `"}`
				}
			}
		}},
		{"private-string-near-envelope", func(r Record) { r.segments[0].items[0].encrypted = strings.Repeat("x", MaxRecordBytes-2048) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := scalarRecord(t, k)
			tc.mutate(r)
			b, e := encodeRecord(r)
			if e != nil {
				t.Fatal("valid boundary did not encode", e)
			}
			decoded, e := decodeRecord(b)
			if e != nil {
				clear(b)
				t.Fatal("accepted boundary failed binary decode", e)
			}
			again, e := encodeRecord(decoded)
			if e != nil || !bytes.Equal(b, again) {
				t.Fatal("boundary codec changed private payload")
			}
			clear(again)
			clear(b)
			decoded.Clear()
			sealed, e := k.seal(r)
			if e != nil {
				t.Fatal("valid boundary did not seal", e)
			}
			opened, e := k.open(sealed)
			clear(sealed)
			if e != nil {
				t.Fatal("accepted boundary failed AEAD reopen", e)
			}
			opened.Clear()
			r.Clear()
		})
	}
}
func TestPublicationRejectsUnreplayableScalars(t *testing.T) {
	for _, kind := range []string{"name", "visible"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			prefix := []atuin.Message{textMessage("user", "synthetic")}
			l, scope := f.lease(t, context.Background(), prefix)
			r := f.build(t, l, scope, prefix, nil)
			if kind == "name" {
				setFunctionName(r, strings.Repeat("n", 257))
			} else {
				setTextLayout(f.store.key, r, strings.Repeat("v", (4<<20)+1), "")
			}
			called := false
			durable, e := f.publish(l, context.Background(), r, func(context.Context) error { called = true; return nil })
			if e == nil || durable || called {
				t.Fatal("unreplayable scalar committed or authorized callback")
			}
		})
	}
}
func TestScalarBoundariesPublishAndResolveAfterRestart(t *testing.T) {
	f := newFixture(t)
	prefix := []atuin.Message{textMessage("user", "synthetic")}
	l, scope := f.lease(t, context.Background(), prefix)
	r := f.build(t, l, scope, prefix, nil)
	client := strings.Repeat("v", 4<<20)
	setTextLayout(f.store.key, r, client, strings.Repeat(" ", 1<<20))
	setFunctionName(r, strings.Repeat("n", 256))
	if d, e := f.publish(l, context.Background(), r, func(context.Context) error { return nil }); !d || e != nil {
		t.Fatal("valid scalar boundary publication rejected", e)
	}
	l.Close()
	messages := history(r.segments[0], prefix, client, false, false)
	if e := f.manager.Close(); e != nil {
		t.Fatal(e)
	}
	disk, e := credentials.OpenStore(f.path)
	if e != nil {
		t.Fatal(e)
	}
	f.disk = disk
	f.open(t)
	fresh, freshScope := f.lease(t, context.Background(), messages)
	loaded, matches, e := fresh.Load(messages, freshScope)
	if e != nil || len(matches) != 1 {
		t.Fatal("valid boundary could not resolve persisted state", e)
	}
	if len(loaded.segments[0].calls[0].name) != 256 || loaded.segments[0].text.bytes != 4<<20 || loaded.segments[0].recipe.rawBytes != 5<<20 {
		t.Fatal("boundary scalar changed on restart")
	}
	loaded.Clear()
}
