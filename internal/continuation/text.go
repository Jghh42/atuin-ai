package continuation

import (
	"crypto/sha256"
	"hash"
	"unicode/utf8"
)

// RustWhiteSpace is the pinned Rust White_Space set, NOT Go TrimSpace or
// trim_end. In particular U+200B and U+FEFF are retained.
func RustWhiteSpace(r rune) bool {
	return r >= 9 && r <= 13 || r == 0x20 || r == 0x85 || r == 0xa0 || r == 0x1680 || r >= 0x2000 && r <= 0x200a || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000
}

type textCapturePayload struct {
	raw, client                hash.Hash
	rawBytes, clientBytes      uint32
	leading                    []byte
	visible, overflow, invalid bool
}

func NewTextCapture() *TextCapture {
	return ownedPointer(newTextCapture(textCapturePayload{raw: sha256.New(), client: sha256.New()}))
}

// Add accepts decoded complete UTF8 deltas, never SSE fragments. It stores no
// per-delta metadata and continues digesting after leading capture overflow.
func (t *TextCapture) Add(s string) error {
	if t == nil || t.textCapturePayload == nil || t.raw == nil || !utf8.ValidString(s) || uint64(t.rawBytes)+uint64(len(s)) > uint64(^uint32(0)) {
		if t != nil && t.textCapturePayload != nil {
			t.invalid = true
		}
		return failure(ProjectionUnsupported)
	}
	_, _ = t.raw.Write([]byte(s))
	t.rawBytes += uint32(len(s))
	offset := 0
	if !t.visible {
		for i, r := range s {
			if !RustWhiteSpace(r) {
				offset = i
				t.visible = true
				break
			}
			offset = i + utf8.RuneLen(r)
		}
		if !t.overflow {
			if len(t.leading)+offset > MaxLeadingBytes {
				t.overflow = true
				clear(t.leading)
				t.leading = nil
			} else {
				t.leading = append(t.leading, s[:offset]...)
			}
		}
	}
	if t.visible {
		_, _ = t.client.Write([]byte(s[offset:]))
		t.clientBytes += uint32(len(s) - offset)
	}
	return nil
}

// Finish verifies final ordinary parts against streamed raw order. Returned
// metadata owns the bounded leading buffer; ordinary text itself is not saved.
func (t *TextCapture) Finish(k *keys, parts []string) (TextExpectation, TextRecipe, error) {
	c, e := t.CaptureLayout(parts)
	if e != nil {
		return TextExpectation{}, TextRecipe{}, e
	}
	return k.textDigest(c.clientBytes, c.clientDigest), c.recipe, nil
}
func (t *TextCapture) CaptureLayout(parts []string) (Captured, error) {
	result := Captured{capturedPayload: &capturedPayload{recipe: newTextRecipe(textRecipePayload{})}}
	if t != nil && t.textCapturePayload != nil {
		result.meaningful = t.visible
	}
	if t == nil || t.textCapturePayload == nil || t.invalid || t.overflow || t.clientBytes > maxVisibleTextBytes || t.rawBytes > maxRawTextBytes || len(parts) > MaxSpans {
		return result, failure(ProjectionUnsupported)
	}
	h := sha256.New()
	var total uint64
	spans := makeSpan(0, len(parts))
	for _, p := range parts {
		if !utf8.ValidString(p) {
			return result, failure(ProjectionUnsupported)
		}
		total += uint64(len(p))
		if total > maxRawTextBytes {
			return result, failure(Limit)
		}
		_, _ = h.Write([]byte(p))
		spans = append(spans, newSpan(spanPayload{bytes: uint32(len(p)), digest: sha256.Sum256([]byte(p))}))
	}
	if total != uint64(t.rawBytes) || !bytesEqual(h.Sum(nil), t.raw.Sum(nil)) {
		return result, failure(ProjectionUnsupported)
	}
	var raw, client [32]byte
	copy(raw[:], t.raw.Sum(nil))
	copy(client[:], t.client.Sum(nil))
	recipe := newTextRecipe(textRecipePayload{leading: t.leading, spans: spans, rawBytes: t.rawBytes, rawDigest: raw})
	clear(t.leading)
	t.leading = nil
	result.recipe = recipe
	result.clientBytes = t.clientBytes
	result.clientDigest = client
	return result, nil
}
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var x byte
	for i := range a {
		x |= a[i] ^ b[i]
	}
	return x == 0
}
func (t *TextCapture) Close() {
	if t == nil || t.textCapturePayload == nil {
		return
	}
	clear(t.leading)
	*t = newTextCapture(textCapturePayload{})
}

// Restore returns raw parts after verifying EVERY span, not just concatenation.
func (r TextRecipe) Restore(client string) ([][]byte, error) {
	if r.textRecipePayload == nil {
		return nil, failure(Corrupt)
	}
	if len(r.leading) > MaxLeadingBytes || len(r.spans) > MaxSpans || len(client) > maxVisibleTextBytes || r.rawBytes > maxRawTextBytes || !utf8.ValidString(client) || !utf8.Valid(r.leading) {
		return nil, failure(Corrupt)
	}
	for _, v := range string(r.leading) {
		if !RustWhiteSpace(v) {
			return nil, failure(Corrupt)
		}
	}
	if client != "" {
		v, _ := utf8.DecodeRuneInString(client)
		if RustWhiteSpace(v) {
			return nil, failure(BindingMismatch)
		}
	}
	if uint64(len(r.leading))+uint64(len(client)) != uint64(r.rawBytes) {
		return nil, failure(Corrupt)
	}
	raw := make([]byte, 0, int(r.rawBytes))
	raw = append(raw, r.leading...)
	raw = append(raw, client...)
	defer clear(raw)
	if sha256.Sum256(raw) != r.rawDigest {
		return nil, failure(BindingMismatch)
	}
	parts := make([][]byte, 0, len(r.spans))
	success := false
	defer func() {
		if !success {
			for _, p := range parts {
				clear(p)
			}
		}
	}()
	offset := uint64(0)
	for _, s := range r.spans {
		if s.spanPayload == nil || s.bytes > maxRawTextBytes {
			return nil, failure(Corrupt)
		}
		end := offset + uint64(s.bytes)
		if end > uint64(len(raw)) {
			return nil, failure(Corrupt)
		}
		b := raw[int(offset):int(end)]
		if !utf8.Valid(b) || sha256.Sum256(b) != s.digest {
			return nil, failure(BindingMismatch)
		}
		parts = append(parts, append([]byte(nil), b...))
		offset = end
	}
	if offset != uint64(len(raw)) {
		return nil, failure(Corrupt)
	}
	success = true
	return parts, nil
}
