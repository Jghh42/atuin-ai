package continuation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/atuin"
)

func sampleRecord(t testing.TB, k *keys, count int, raw string, prefixMessages []atuin.Message, seed byte) Record {
	t.Helper()
	scope := keyed(k.derive("binding"), "test-scope")
	id := newSnapshotID(snapshotIDPayload{})
	id.bytes[0] = seed
	now := int64(1793491200000)
	prefix, e := k.prefix(prefixMessages)
	if e != nil {
		t.Fatal(e)
	}
	capture := NewTextCapture()
	_ = capture.Add(raw)
	var texts []string
	if raw != "" {
		texts = []string{raw}
	}
	expected, recipe, e := capture.Finish(k, texts)
	if e != nil {
		t.Fatal(e)
	}
	s := newSegment(segmentPayload{snapshot: id, created: now, expires: now + TTLMillis, prefix: prefix, text: expected, recipe: recipe, items: []Item{newItem(itemPayload{kind: 3, iD: fmt.Sprintf("reasoning-%d", seed), encryptedPresence: 2, encrypted: "PRIVATE-CIPHER", summary: []Part{newPart(partPayload{kind: 2, text: "PRIVATE-SUMMARY"})}})}})
	if raw != "" {
		s.items = append(s.items, newItem(itemPayload{kind: 1, iD: fmt.Sprintf("message-%d", seed), status: "completed", statusPresence: 2, contentPresence: 2, content: []Part{newPart(partPayload{kind: 1})}}))
	}
	for j := 0; j < count; j++ {
		h, e := k.handle(id, uint8(j), scope)
		if e != nil {
			t.Fatal(e)
		}
		args := fmt.Sprintf(`{"file_path":"synthetic-%d","n":%d}`, seed, j)
		digest, e := k.arguments([]byte(args), true)
		if e != nil {
			t.Fatal(e)
		}
		backend := fmt.Sprintf("backend-%d-%d", seed, j)
		s.calls = append(s.calls, newCall(callPayload{handle: h, backend: newBackendCallID(backendCallIDPayload{value: backend}), name: "read_file", publicArguments: digest}))
		s.items = append(s.items, newItem(itemPayload{kind: 2, iD: fmt.Sprintf("item-%d-%d", seed, j), callID: backend, name: "read_file", arguments: args}))
	}
	return newRecord(recordPayload{snapshot: id, scope: scope, created: now, expires: now + TTLMillis, segments: []Segment{s}})
}
func callBlock(s Segment, j int) atuin.Block {
	c := s.calls[j]
	args := ""
	for _, i := range s.items {
		if i.callID == c.backend.value {
			args = i.arguments
		}
	}
	return atuin.Block{Type: "tool_use", ID: c.handle.value, Name: c.name, Input: json.RawMessage(args)}
}
func resultMessage(s Segment, j int) atuin.Message {
	return atuin.Message{Role: "user", Content: []atuin.Block{{Type: "tool_result", ToolUseID: s.calls[j].handle.value, Content: fmt.Sprintf("synthetic-result-%d", j)}}}
}
func history(s Segment, prefix []atuin.Message, client string, interleave, reverse bool) []atuin.Message {
	out := append([]atuin.Message(nil), prefix...)
	run := atuin.Message{Role: "assistant"}
	if client != "" {
		run.Content = append(run.Content, atuin.Block{Type: "text", Text: client})
	}
	for j := range s.calls {
		run.Content = append(run.Content, callBlock(s, j))
		if interleave {
			out = append(out, run, resultMessage(s, j))
			run = atuin.Message{Role: "assistant"}
		}
	}
	if !interleave {
		out = append(out, run)
		for j := range s.calls {
			n := j
			if reverse {
				n = len(s.calls) - j - 1
			}
			out = append(out, resultMessage(s, n))
		}
	}
	return out
}
func TestCompleteBatchInterleavingAndPrefix(t *testing.T) {
	k := testKeys()
	prefix := []atuin.Message{textMessage("user", "original")}
	for count := 1; count <= 32; count++ {
		r := sampleRecord(t, k, count, " \nanswer \n", prefix, 1)
		for _, mode := range []struct{ interleave, reverse bool }{{true, false}, {false, false}, {false, true}} {
			messages := history(r.segments[0], prefix, "answer \n", mode.interleave, mode.reverse)
			messages = append(messages, textMessage("user", "next"))
			matches, e := k.verifyRecord(r, messages, r.scope)
			if e != nil || len(matches) != 1 || len(matches[0].results) != count || matches[0].end != len(messages)-1 {
				t.Fatal("valid released grammar rejected", count, e)
			}
		}
	}
	r := sampleRecord(t, k, 2, "", prefix, 1)
	s := r.segments[0]
	// The normalizer accepts the shortened A/resultA history. It has no record of
	// issued B. The sealed independent K contract MUST reject it.
	short := history(s, prefix, "", true, false)[:3]
	wire := fmt.Sprintf(`{"messages":[{"role":"user","content":"original"},{"role":"assistant","content":[{"type":"tool_use","id":%q,"name":"read_file","input":{"file_path":"x"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]}]}`, s.calls[0].handle.value, s.calls[0].handle.value)
	decoder := atuin.Decoder{Catalog: atuin.ModelCatalog{Default: "default", Models: map[string]string{"default": "model"}}}
	if _, e := decoder.Decode([]byte(wire)); e != nil {
		t.Fatal("normalizer premise failed", e)
	}
	if _, e := k.verifyRecord(r, short, r.scope); e == nil {
		t.Fatal("missing B AND resultB accepted")
	}
	valid := history(s, prefix, "", false, false)
	mutations := [][]atuin.Message{
		{prefix[0], resultMessage(s, 0), valid[1], resultMessage(s, 1)},
		{prefix[0], {Role: "assistant", Content: []atuin.Block{callBlock(s, 1), callBlock(s, 0)}}, resultMessage(s, 0), resultMessage(s, 1)},
		{prefix[0], {Role: "assistant", Content: []atuin.Block{callBlock(s, 0)}}, {Role: "assistant", Content: []atuin.Block{callBlock(s, 1)}}, resultMessage(s, 0), resultMessage(s, 1)},
		{prefix[0], valid[1], resultMessage(s, 0), resultMessage(s, 0)},
		{textMessage("user", "edited"), valid[1], valid[2], valid[3]},
	}
	for _, m := range mutations {
		if _, e := k.verifyRecord(r, m, r.scope); e == nil {
			t.Fatal("invalid sequence/prefix accepted")
		}
	}
	// The public window marker is ordinary user text, NOT a bypass or reset.
	changed := append([]atuin.Message(nil), valid...)
	changed[0] = textMessage("user", "[Earlier conversation context was omitted to fit within the context window. The conversation continues below.]")
	if _, e := k.verifyRecord(r, changed, r.scope); e == nil {
		t.Fatal("windowed prefix accepted")
	}
}
func TestSelfContainedAncestryAndAllMembership(t *testing.T) {
	k := testKeys()
	prefix := []atuin.Message{textMessage("user", "original")}
	first := sampleRecord(t, k, 2, "", prefix, 1)
	messages := history(first.segments[0], prefix, "", true, false)
	messages = append(messages, textMessage("user", "next"))
	second := sampleRecord(t, k, 1, "", messages, 2)
	second.segments = append(first.segments, second.segments...)
	second.expires = first.expires
	messages = history(second.segments[1], messages, "", false, false)
	if _, e := k.verifyRecord(second, messages, second.scope); e != nil {
		t.Fatal("self-contained ancestry", e)
	}
	b, e := encodeRecord(second)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := decodeRecord(b)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := k.verifyRecord(decoded, messages, second.scope); e != nil {
		t.Fatal(e)
	}
	messages[1].Content[0].ID = messages[1].Content[0].ID[:89] + "!"
	if _, e := k.verifyRecord(decoded, messages, second.scope); e == nil {
		t.Fatal("old member tamper ignored")
	}
}
func TestCodecBoundsTamperAndCiphertext(t *testing.T) {
	k := testKeys()
	r := sampleRecord(t, k, 1, "\ttext \n", []atuin.Message{textMessage("user", "unpersisted-prompt")}, 1)
	b, e := k.seal(r)
	if e != nil {
		t.Fatal(e)
	}
	for _, marker := range []string{"PRIVATE-CIPHER", "PRIVATE-SUMMARY", "backend-1-0", "unpersisted-prompt", "synthetic-1"} {
		if stringContains(b, marker) {
			t.Fatal("plaintext sealed marker")
		}
	}
	for _, offset := range []int{0, 8, 16, 32, 64, 72, 80, 91, 92, len(b) - 1} {
		bad := append([]byte(nil), b...)
		bad[offset] ^= 1
		if _, e := k.open(bad); e == nil {
			t.Fatal("AAD/nonce/payload tamper accepted")
		}
	}
	got, e := k.open(b)
	if e != nil || len(got.segments) != 1 {
		t.Fatal(e)
	}
	got.Clear()
	payload, e := encodeRecord(r)
	if e != nil {
		t.Fatal(e)
	}
	for n := 0; n < len(payload); n++ {
		if _, e := decodeRecord(payload[:n]); e == nil {
			t.Fatal("truncation accepted")
		}
	}
	huge := append([]byte(nil), payload...) // segment count follows length-prefixed magic + fixed metadata
	for j := 8 + len(recordMagic) + 16 + 32 + 8 + 8; j < 8+len(recordMagic)+16+32+8+8+8; j++ {
		huge[j] = 255
	}
	if _, e := decodeRecord(huge); e == nil {
		t.Fatal("collection allocation bound bypass")
	}
}
func stringContains(b []byte, s string) bool {
	for j := 0; j+len(s) <= len(b); j++ {
		if string(b[j:j+len(s)]) == s {
			return true
		}
	}
	return false
}
func FuzzPrivateCodec(f *testing.F) {
	f.Add([]byte(recordMagic))
	valid, e := encodeRecord(sampleRecord(f, testKeys(), 2, " \ttext ", []atuin.Message{textMessage("user", "synthetic")}, 1))
	if e != nil {
		f.Fatal(e)
	}
	f.Add(valid)
	near := scalarRecord(f, testKeys())
	setFunctionName(near, strings.Repeat("n", 256))
	setTextLayout(testKeys(), near, strings.Repeat("v", 4<<20), "")
	near.segments[0].prefix.messages = 65536
	nearBytes, e := encodeRecord(near)
	if e != nil {
		f.Fatal(e)
	}
	f.Add(nearBytes)
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > MaxRecordBytes {
			return
		}
		r, e := decodeRecord(b)
		if e == nil {
			encoded, e := encodeRecord(r)
			if e != nil {
				t.Fatal("accepted invalid record", e)
			}
			again, e := decodeRecord(encoded)
			if e != nil {
				t.Fatal("accepted encoding cannot decode", e)
			}
			reencoded, e := encodeRecord(again)
			if e != nil || !bytes.Equal(encoded, reencoded) {
				t.Fatal("codec changed private payload")
			}
			clear(encoded)
			clear(reencoded)
			again.Clear()
			k := testKeys()
			sealed, e := k.seal(r)
			if e != nil {
				t.Fatal("accepted record cannot seal", e)
			}
			opened, e := k.open(sealed)
			clear(sealed)
			if e != nil {
				t.Fatal("accepted seal cannot reopen", e)
			}
			opened.Clear()
			r.Clear()
		}
	})
}
