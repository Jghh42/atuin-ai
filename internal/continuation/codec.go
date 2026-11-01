package continuation

import (
	"encoding/binary"
	"unicode/utf8"
)

// This is the ONLY private-record serialization path. JSON/fmt/slog remain
// redacted even for nested records. The finite binary grammar has fixed depth
// (<8); sizes/work are checked BEFORE collection allocation or string copying.
type encoderPayload struct {
	b    []byte
	work int
	bad  bool
}

func (w *encoder) raw(b []byte) {
	if w.bad || w.work >= MaxTokens || len(b) > maxRecordPayloadBytes-len(w.b) {
		w.bad = true
		return
	}
	w.work++
	w.b = append(w.b, b...)
}
func (w *encoder) u(n uint64)    { var b [8]byte; binary.BigEndian.PutUint64(b[:], n); w.raw(b[:]) }
func (w *encoder) data(b []byte) { w.u(uint64(len(b))); w.raw(b) }
func (w *encoder) str(s string) {
	if w.bad || !utf8.ValidString(s) || len(s) > maxRecordPayloadBytes-len(w.b)-8 {
		w.bad = true
		return
	}
	w.u(uint64(len(s)))
	if w.bad || w.work >= MaxTokens {
		w.bad = true
		return
	}
	w.work++ // Match decoder's separate length/data structural charges.
	w.b = append(w.b, s...)
}

type decoderPayload struct {
	b         []byte
	pos, work int
	bad       bool
}

func (d *decoder) raw(n int) []byte {
	if d.bad || n < 0 || n > len(d.b)-d.pos || d.work >= MaxTokens {
		d.bad = true
		return nil
	}
	d.work++
	v := d.b[d.pos : d.pos+n]
	d.pos += n
	return v
}
func (d *decoder) u() uint64 {
	b := d.raw(8)
	if len(b) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
func (d *decoder) count(max int) int {
	if max < 0 {
		d.bad = true
		return 0
	}
	n := d.u()
	if n > uint64(max) || n > uint64(len(d.b)-d.pos) {
		d.bad = true
		return 0
	}
	return int(n)
}
func (d *decoder) data(max int) []byte { n := d.count(max); return d.raw(n) }
func (d *decoder) str(max int) string {
	v := d.data(max)
	if !utf8.Valid(v) {
		d.bad = true
		return ""
	}
	return string(v)
}
func encodeRecord(r Record) ([]byte, error) {
	if validateRecord(r) != nil {
		return nil, failure(Corrupt)
	}
	w := newEncoder(encoderPayload{})
	w.str(recordMagic)
	w.raw(r.snapshot.bytes[:])
	w.raw(r.scope.bytes[:])
	w.u(uint64(r.created))
	w.u(uint64(r.expires))
	w.u(uint64(len(r.segments)))
	parts := func(ps []Part) {
		w.u(uint64(len(ps)))
		for _, p := range ps {
			w.u(uint64(p.kind))
			w.str(p.text)
			w.u(uint64(p.span))
			w.u(uint64(p.logprobsPresence))
		}
	}
	for _, s := range r.segments {
		w.raw(s.snapshot.bytes[:])
		w.u(uint64(s.created))
		w.u(uint64(s.expires))
		w.u(uint64(s.prefix.messages))
		w.raw(s.prefix.digest.bytes[:])
		w.u(uint64(s.text.bytes))
		w.raw(s.text.digest.bytes[:])
		w.data(s.recipe.leading)
		w.u(uint64(s.recipe.rawBytes))
		w.raw(s.recipe.rawDigest[:])
		w.u(uint64(len(s.recipe.spans)))
		for _, p := range s.recipe.spans {
			w.u(uint64(p.bytes))
			w.raw(p.digest[:])
		}
		w.u(uint64(len(s.calls)))
		for _, c := range s.calls {
			w.str(c.handle.value)
			w.str(c.backend.value)
			w.str(c.name)
			w.raw(c.publicArguments.bytes[:])
		}
		w.u(uint64(len(s.items)))
		for _, i := range s.items {
			w.u(uint64(i.kind))
			w.str(i.iD)
			w.str(i.callID)
			w.str(i.name)
			w.str(i.arguments)
			w.str(i.encrypted)
			w.str(i.status)
			w.u(uint64(i.contentPresence))
			w.u(uint64(i.encryptedPresence))
			w.u(uint64(i.statusPresence))
			w.str(i.phase)
			w.u(uint64(i.phasePresence))
			w.u(uint64(i.callerPresence))
			w.u(uint64(i.asyncPresence))
			parts(i.summary)
			parts(i.content)
		}
	}
	if w.bad {
		clear(w.b)
		return nil, failure(Limit)
	}
	return w.b, nil
}
func decodeRecord(b []byte) (Record, error) {
	var r Record = newRecord(recordPayload{})
	if len(b) > maxRecordPayloadBytes {
		return r, failure(Limit)
	}
	d := newDecoder(decoderPayload{b: b})
	defer clear(d.b)
	if d.str(len(recordMagic)) != recordMagic {
		return r, failure(Corrupt)
	}
	copy(r.snapshot.bytes[:], d.raw(16))
	copy(r.scope.bytes[:], d.raw(32))
	r.created = int64(d.u())
	r.expires = int64(d.u())
	n := d.count(MaxSegments)
	r.segments = makeSegment(n)
	totalItems, totalParts, totalSpans := 0, 0, 0
	parts := func() []Part {
		n := d.count(MaxParts - totalParts)
		totalParts += n
		ps := makePart(n)
		for j := range ps {
			kind := d.u()
			if kind > maxPartKind {
				d.bad = true
			}
			ps[j].kind = uint8(kind)
			ps[j].text = d.str(maxPartTextBytes)
			span := d.u()
			if span > MaxSpans {
				d.bad = true
			}
			ps[j].span = uint32(span)
			v := d.u()
			if v > maxPresence {
				d.bad = true
			}
			ps[j].logprobsPresence = uint8(v)
		}
		return ps
	}
	for j := range r.segments {
		s := &r.segments[j]
		copy(s.snapshot.bytes[:], d.raw(16))
		s.created = int64(d.u())
		s.expires = int64(d.u())
		v := d.u()
		if v > maxPrefixMessages {
			d.bad = true
		}
		s.prefix.messages = uint32(v)
		copy(s.prefix.digest.bytes[:], d.raw(32))
		v = d.u()
		if v > maxVisibleTextBytes {
			d.bad = true
		}
		s.text.bytes = uint32(v)
		copy(s.text.digest.bytes[:], d.raw(32))
		s.recipe.leading = append([]byte(nil), d.data(MaxLeadingBytes)...)
		v = d.u()
		if v > maxRawTextBytes {
			d.bad = true
		}
		s.recipe.rawBytes = uint32(v)
		copy(s.recipe.rawDigest[:], d.raw(32))
		n := d.count(MaxSpans - totalSpans)
		totalSpans += n
		s.recipe.spans = makeSpan(n)
		for j := range s.recipe.spans {
			v := d.u()
			if v > maxRawTextBytes {
				d.bad = true
			}
			s.recipe.spans[j].bytes = uint32(v)
			copy(s.recipe.spans[j].digest[:], d.raw(32))
		}
		n = d.count(MaxCalls)
		s.calls = makeCall(n)
		for j := range s.calls {
			c := &s.calls[j]
			c.handle.value = d.str(HandleBytes)
			c.backend.value = d.str(maxIDBytes)
			c.name = d.str(maxNameBytes)
			copy(c.publicArguments.bytes[:], d.raw(32))
		}
		n = d.count(min(maxOutputItems, MaxItems-totalItems))
		totalItems += n
		s.items = makeItem(n)
		for j := range s.items {
			i := &s.items[j]
			kind := d.u()
			if kind > maxItemKind {
				d.bad = true
			}
			i.kind = uint8(kind)
			i.iD = d.str(maxIDBytes)
			i.callID = d.str(maxIDBytes)
			i.name = d.str(maxNameBytes)
			i.arguments = d.str(maxArgumentsBytes)
			i.encrypted = d.str(maxEncryptedBytes)
			i.status = d.str(maxStatusBytes)
			v := d.u()
			if v > maxPresence {
				d.bad = true
			}
			i.contentPresence = uint8(v)
			v = d.u()
			if v > maxPresence {
				d.bad = true
			}
			i.encryptedPresence = uint8(v)
			v = d.u()
			if v > maxPresence {
				d.bad = true
			}
			i.statusPresence = uint8(v)
			i.phase = d.str(maxPhaseBytes)
			v = d.u()
			if v > maxPresence {
				d.bad = true
			}
			i.phasePresence = uint8(v)
			v = d.u()
			if v > maxPresence {
				d.bad = true
			}
			i.callerPresence = uint8(v)
			v = d.u()
			if v > maxPresence {
				d.bad = true
			}
			i.asyncPresence = uint8(v)
			i.summary = parts()
			i.content = parts()
		}
	}
	if d.bad || d.pos != len(b) || validateRecord(r) != nil {
		r.Clear()
		return newRecord(recordPayload{}), failure(Corrupt)
	}
	return r, nil
}
func validTimes(created, expires int64) bool {
	return created >= minMillis && created <= maxMillis-TTLMillis && expires > created && expires <= created+TTLMillis && expires <= maxMillis
}
func validateRecord(r Record) error {
	if r.recordPayload == nil || r.snapshot.snapshotIDPayload == nil || r.scope.commitmentPayload == nil {
		return failure(Corrupt)
	}
	if len(r.segments) > 0 {
		last := r.segments[len(r.segments)-1]
		if last.segmentPayload == nil {
			return failure(Corrupt)
		}
		if e := boundedInputs(r.segments[:len(r.segments)-1], last.items, last.calls, last.recipe); e != nil {
			return e
		}
	}
	if !validTimes(r.created, r.expires) || len(r.segments) == 0 || len(r.segments) > MaxSegments {
		return failure(Corrupt)
	}
	items, parts, spans := 0, 0, 0
	seenItems := map[string]bool{}
	seenBackend := map[string]bool{}
	seenSnapshots := map[[16]byte]bool{}
	minimum := r.created + TTLMillis
	lastCreated := int64(0)
	for _, s := range r.segments {
		if s.snapshot.snapshotIDPayload == nil || s.prefix.prefixPayload == nil || s.prefix.digest.commitmentPayload == nil || s.text.textExpectationPayload == nil || s.text.digest.commitmentPayload == nil {
			return failure(Corrupt)
		}
		if s.prefix.messages > maxPrefixMessages || s.text.bytes > maxVisibleTextBytes {
			return failure(Limit)
		}
		if !validTimes(s.created, s.expires) || s.created < lastCreated || s.created > r.created || seenSnapshots[s.snapshot.bytes] || len(s.calls) == 0 || len(s.calls) > MaxCalls || len(s.recipe.leading) > MaxLeadingBytes || !utf8.Valid(s.recipe.leading) {
			return failure(Corrupt)
		}
		seenSnapshots[s.snapshot.bytes] = true
		lastCreated = s.created
		if s.expires < minimum {
			minimum = s.expires
		}
		spans += len(s.recipe.spans)
		items += len(s.items)
		if spans > MaxSpans || items > MaxItems {
			return failure(Limit)
		}
		var raw uint64
		for _, p := range s.recipe.spans {
			raw += uint64(p.bytes)
		}
		if raw != uint64(s.recipe.rawBytes) || raw != uint64(len(s.recipe.leading))+uint64(s.text.bytes) || raw > maxRawTextBytes {
			return failure(Corrupt)
		}
		for _, v := range string(s.recipe.leading) {
			if !RustWhiteSpace(v) {
				return failure(Corrupt)
			}
		}
		calls := map[string]Call{}
		for ordinal, c := range s.calls {
			if c.publicArguments.commitmentPayload == nil {
				return failure(Corrupt)
			}
			p, e := parseHandle(c.handle.Value())
			if e != nil || !sameSnapshot(p.snapshot, s.snapshot) || int(p.ordinal) != ordinal || c.backend.Value() == "" || Reserved(c.backend.Value()) || seenBackend[c.backend.value] {
				return failure(Corrupt)
			}
			seenBackend[c.backend.value] = true
			calls[c.backend.value] = c
		}
		used := map[string]bool{}
		spanOrdinal := 0
		for _, i := range s.items {
			if i.iD == "" || seenItems[i.iD] || i.contentPresence > maxPresence || i.encryptedPresence > maxPresence || i.statusPresence > maxPresence {
				return failure(Corrupt)
			}
			seenItems[i.iD] = true
			parts += len(i.summary) + len(i.content)
			if parts > MaxParts {
				return failure(Limit)
			}
			if i.contentPresence != 2 && len(i.content) != 0 || i.encryptedPresence != 2 && i.encrypted != "" || i.statusPresence != 2 && i.status != "" || i.statusPresence == 2 && i.status != "completed" {
				return failure(Corrupt)
			}
			switch i.kind {
			case 1:
				if i.statusPresence != 2 || i.status != "completed" || i.callerPresence != 0 || i.asyncPresence != 0 || i.phasePresence != 2 && i.phase != "" || i.phasePresence == 2 && i.phase != "commentary" && i.phase != "final_answer" {
					return failure(Corrupt)
				}
				if len(i.summary) != 0 || i.encryptedPresence != 0 || i.callID != "" || i.name != "" || i.arguments != "" {
					return failure(Corrupt)
				}
				for _, p := range i.content {
					if p.kind != 1 || p.text != "" || int(p.span) != spanOrdinal || spanOrdinal >= len(s.recipe.spans) {
						return failure(Corrupt)
					}
					spanOrdinal++
				}
			case 2:
				if i.phasePresence != 0 || i.phase != "" {
					return failure(Corrupt)
				}
				c, ok := calls[i.callID]
				if !ok || used[i.callID] || i.name != c.name || len(i.content) != 0 || len(i.summary) != 0 || i.encryptedPresence != 0 {
					return failure(Corrupt)
				}
				used[i.callID] = true
				if _, e := CanonicalArguments([]byte(i.arguments), true); e != nil {
					return e
				}
			case 3:
				if i.phasePresence != 0 || i.phase != "" || i.callerPresence != 0 || i.asyncPresence != 0 {
					return failure(Corrupt)
				}
				if i.encryptedPresence != 2 || i.encrypted == "" || i.callID != "" || i.name != "" || i.arguments != "" {
					return failure(ReasoningStateMissing)
				}
				for _, p := range i.summary {
					if p.kind != 2 || p.span != 0 || p.logprobsPresence != 0 {
						return failure(Corrupt)
					}
				}
				for _, p := range i.content {
					if p.kind != 3 || p.span != 0 || p.logprobsPresence != 0 {
						return failure(Corrupt)
					}
				}
			default:
				return failure(Corrupt)
			}
		}
		if len(used) != len(calls) || spanOrdinal != len(s.recipe.spans) {
			return failure(Corrupt)
		}
	}
	if minimum != r.expires || !sameSnapshot(r.segments[len(r.segments)-1].snapshot, r.snapshot) {
		return failure(Corrupt)
	}
	return nil
}

// Clear is best-effort owned-memory release, NOT secure erasure of Go strings,
// copies, filesystem blocks, SSDs or backups.
func (r *Record) Clear() {
	if r == nil || r.recordPayload == nil {
		return
	}
	for j := range r.segments {
		clear(r.segments[j].recipe.leading)
		r.segments[j] = newSegment(segmentPayload{})
	}
	*r = newRecord(recordPayload{})
}
