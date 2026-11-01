package codex

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"math"
	"strings"

	"github.com/Jghh42/atuin-ai/internal/continuation"
	"github.com/Jghh42/atuin-ai/internal/provider"
)

// All presentations contain only unexported pointers. Invalid fmt verbs bypass
// Formatter methods in Go1.27; backing decoder data is trusted codec access only.
type privateValue struct{}

func (privateValue) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "[private generation]") }
func (privateValue) MarshalJSON() ([]byte, error) { return []byte(`"[private generation]"`), nil }

type reasoning struct {
	privateValue
	*reasoningPayload
}
type reasoningPayload struct {
	id                string
	summary, content  []string
	cp, ep, sp        continuation.Presence
	encrypted, status string
	digest            [32]byte
}

func (*reasoning) LogValue() slog.Value { return slog.StringValue("[private generation]") }

type rawReasoning struct {
	privateValue
	*rawReasoningPayload
}
type rawReasoningPayload struct{ data []byte }

func (*rawReasoning) LogValue() slog.Value { return slog.StringValue("[private generation]") }

type reasoningSlot struct {
	privateValue
	*reasoningSlotPayload
}
type reasoningSlotPayload struct {
	id           string
	added, final [32]byte
	done         bool
}

func (*reasoningSlot) LogValue() slog.Value { return slog.StringValue("[private generation]") }

type itemMetadata struct {
	privateValue
	*itemMetadataPayload
}
type itemMetadataPayload struct {
	status, phase                                                            string
	presence, phasePresence, callerPresence, asyncPresence, logprobsPresence continuation.Presence
	unsupported, sticky, executionInvalid                                    bool
	digest                                                                   [32]byte
}

func (*itemMetadata) LogValue() slog.Value { return slog.StringValue("[private generation]") }

type completion struct {
	privateValue
	*completionPayload
}
type completionPayload struct {
	items                                        []continuation.Item
	capture                                      continuation.Captured
	projectionUnsupported, handled, missingState bool
}

func (*completion) LogValue() slog.Value { return slog.StringValue("[private generation]") }
func (c *completion) close() {
	if c == nil || c.completionPayload == nil {
		return
	}
	c.capture.Clear()
	c.items = nil
}

func strictObject(raw []byte, allowed ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return nil, ErrStream
	}
	v := make(map[string]json.RawMessage, len(allowed))
	for d.More() {
		key, e := d.Token()
		name, ok := key.(string)
		if e != nil || !ok {
			return nil, ErrStream
		}
		found := false
		for _, a := range allowed {
			if a == name {
				found = true
				break
			}
		}
		if !found || v[name] != nil {
			return nil, ErrStream
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, ErrStream
		}
		v[name] = value
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim('}') {
		return nil, ErrStream
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, ErrStream
	}
	return v, nil
}
func requiredString(v map[string]json.RawMessage, key string) (string, error) {
	var s string
	b, ok := v[key]
	if !ok || len(b) == 0 || b[0] != '"' || json.Unmarshal(b, &s) != nil {
		return "", ErrStream
	}
	return s, nil
}
func optionalString(v map[string]json.RawMessage, key string) (string, continuation.Presence, error) {
	b, ok := v[key]
	if !ok {
		return "", continuation.Omitted, nil
	}
	if bytes.Equal(b, []byte("null")) {
		return "", continuation.Null, nil
	}
	s, e := requiredString(v, key)
	return s, continuation.Present, e
}
func readPart(raw []byte, kind string) (string, error) {
	v, e := strictObject(raw, "type", "text")
	if e != nil {
		return "", e
	}
	typ, e := requiredString(v, "type")
	if e != nil || typ != kind {
		return "", ErrStream
	}
	return requiredString(v, "text")
}
func readParts(raw json.RawMessage, kind string, limit int) ([]string, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('[') {
		return nil, ErrStream
	}
	out := make([]string, 0)
	for d.More() {
		if len(out) >= limit {
			return nil, ErrStream
		}
		var part json.RawMessage
		if d.Decode(&part) != nil {
			return nil, ErrStream
		}
		text, e := readPart(part, kind)
		if e != nil {
			return nil, e
		}
		out = append(out, text)
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim(']') {
		return nil, ErrStream
	}
	return out, nil
}
func digestField(h hash.Hash, s string) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(len(s)))
	_, _ = h.Write(b[:])
	_, _ = h.Write([]byte(s))
}
func readReasoning(raw []byte) (*reasoning, error) {
	v, e := strictObject(raw, "id", "type", "summary", "content", "encrypted_content", "status")
	if e != nil {
		return nil, e
	}
	id, e := requiredString(v, "id")
	if e != nil || !identifier(id) {
		return nil, ErrStream
	}
	typ, e := requiredString(v, "type")
	if e != nil || typ != "reasoning" {
		return nil, ErrStream
	}
	summary, e := readParts(v["summary"], "summary_text", maxOutputItems)
	if e != nil {
		return nil, e
	}
	p := &reasoningPayload{id: id, summary: summary}
	if b, ok := v["content"]; ok {
		p.cp = continuation.Null
		if !bytes.Equal(b, []byte("null")) {
			p.cp = continuation.Present
			p.content, e = readParts(b, "reasoning_text", maxOutputItems-len(summary))
			if e != nil {
				return nil, e
			}
		}
	}
	p.encrypted, p.ep, e = optionalString(v, "encrypted_content")
	if e != nil {
		return nil, e
	}
	p.status, p.sp, e = optionalString(v, "status")
	if e != nil {
		return nil, e
	}
	if p.sp == continuation.Present && p.status != "in_progress" && p.status != "completed" && p.status != "incomplete" {
		return nil, ErrStream
	}
	h := sha256.New()
	digestField(h, id)
	for _, parts := range [][]string{p.summary, p.content} {
		digestField(h, string(binary.BigEndian.AppendUint64(nil, uint64(len(parts)))))
		for _, text := range parts {
			digestField(h, text)
		}
	}
	digestField(h, string([]byte{byte(p.cp), byte(p.ep), byte(p.sp)}))
	digestField(h, p.encrypted)
	digestField(h, p.status)
	copy(p.digest[:], h.Sum(nil))
	return &reasoning{reasoningPayload: p}, nil
}
func (i *outputItem) UnmarshalJSON(raw []byte) error {
	type plain outputItem
	var decoded plain
	if json.Unmarshal(raw, &decoded) != nil {
		return ErrStream
	}
	if decoded.Type == "reasoning" {
		*i = outputItem{Type: decoded.Type, rawReason: &rawReasoning{rawReasoningPayload: &rawReasoningPayload{data: append([]byte(nil), raw...)}}}
		return nil
	}
	*i = outputItem(decoded)
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil {
		return ErrStream
	}
	i.metadata = outputMetadata(v, i.Type)
	return nil
}
func (p *parser) reasonItem(index int, item outputItem, final bool) (*outputSlot, error) {
	if !p.prepared {
		return nil, ErrUnsupported
	}
	if index < 0 || index >= maxOutputItems || item.rawReason == nil {
		return nil, ErrStream
	}
	r, e := readReasoning(item.rawReason.data)
	if e != nil {
		return nil, e
	}
	if r.sp == continuation.Present && r.status == "incomplete" {
		return nil, failure(ErrResponse, provider.PhaseStream, provider.UpstreamIncomplete)
	}
	if final && r.sp == continuation.Present && r.status != "completed" {
		return nil, ErrStream
	}
	s := p.slots[index]
	if s == nil {
		for _, other := range p.slots {
			if slotID(other) == r.id {
				return nil, ErrStream
			}
		}
		s = &outputSlot{item: outputItem{Type: "reasoning"}, texts: map[int]*textState{}, reason: &reasoningSlot{reasoningSlotPayload: &reasoningSlotPayload{id: strings.Clone(r.id), added: r.digest}}}
		p.slots[index] = s
	} else if s.reason == nil || slotID(s) != r.id {
		return nil, ErrStream
	} else if !final {
		if s.reason.added != r.digest {
			return nil, ErrStream
		}
		return s, nil
	}
	if final && s.reason.done {
		if s.reason.final != r.digest {
			return nil, ErrStream
		}
		return s, nil
	}
	if final {
		for key := range s.texts {
			index := key / 2
			if key%2 == 0 && index >= len(r.summary) || key%2 == 1 && index >= len(r.content) {
				return nil, ErrStream
			}
		}
	}
	for channel, parts := range [][]string{r.summary, r.content} {
		for j, text := range parts {
			var e error
			if final {
				e = p.hiddenText(s, j, channel, text, true)
			} else {
				e = p.hiddenSnapshot(s, j, channel, text)
			}
			if e != nil {
				return nil, e
			}
		}
	}
	if final {
		s.reason.final = r.digest
		s.reason.done = true
	} else {
		s.reason.id = strings.Clone(r.id)
	}
	return s, nil
}
func slotID(s *outputSlot) string {
	if s.reason != nil {
		return s.reason.id
	}
	return s.item.ID
}
func (p *parser) hiddenText(s *outputSlot, index, channel int, text string, final bool) error {
	if index < 0 || index >= maxOutputItems || channel < 0 || channel > 1 {
		return ErrStream
	}
	key := index*2 + channel
	state := s.texts[key]
	if state == nil {
		if p.textCount >= maxOutputItems {
			return ErrStream
		}
		p.textCount++
		state = &textState{sum: sha256.New()}
		s.texts[key] = state
	}
	if final {
		if state.size == 0 && !state.done {
			_, _ = state.sum.Write([]byte(text))
			state.size = int64(len(text))
		}
		sum := sha256.Sum256([]byte(text))
		if state.size != int64(len(text)) || !bytes.Equal(state.sum.Sum(nil), sum[:]) {
			return ErrStream
		}
		state.done = true
		return nil
	}
	if state.done || state.size > math.MaxInt64-int64(len(text)) {
		return ErrStream
	}
	_, _ = state.sum.Write([]byte(text))
	state.size += int64(len(text))
	return nil
}
func (p *parser) hiddenSnapshot(s *outputSlot, index, channel int, text string) error {
	if index < 0 || index >= maxOutputItems || channel < 0 || channel > 1 {
		return ErrStream
	}
	if state := s.texts[index*2+channel]; state != nil {
		if state.done {
			return ErrStream
		}
		if text == "" {
			return nil
		}
		if state.size > int64(len(text)) {
			return ErrStream
		}
		sum := sha256.Sum256([]byte(text[:state.size]))
		if !bytes.Equal(state.sum.Sum(nil), sum[:]) {
			return ErrStream
		}
		return p.hiddenText(s, index, channel, text[state.size:], false)
	}
	return p.hiddenText(s, index, channel, text, false)
}
func (*parser) LogValue() slog.Value { return slog.StringValue("[private generation]") }
func (p *parser) reasonEvent(e wireEvent, data []byte) error {
	if !p.prepared {
		return ErrUnsupported
	}
	partEvent := e.Type == "response.reasoning_summary_part.added" || e.Type == "response.reasoning_summary_part.done"
	summary := partEvent || e.Type == "response.reasoning_summary_text.delta" || e.Type == "response.reasoning_summary_text.done"
	allowed := []string{"type", "sequence_number", "item_id", "output_index", "response_id"}
	if summary {
		allowed = append(allowed, "summary_index")
	} else {
		allowed = append(allowed, "content_index")
	}
	if partEvent {
		allowed = append(allowed, "part")
		if e.Type == "response.reasoning_summary_part.done" {
			allowed = append(allowed, "status")
		}
	} else if e.Type == "response.reasoning_text.delta" || e.Type == "response.reasoning_summary_text.delta" {
		allowed = append(allowed, "delta")
	} else {
		allowed = append(allowed, "text")
	}
	v, err := strictObject(data, allowed...)
	if err != nil || e.Sequence == nil {
		return ErrStream
	}
	s, err := p.slot(e, "reasoning")
	if err != nil {
		return err
	}
	if s.reason.done {
		return ErrStream
	}
	index := e.ContentIndex
	if summary {
		index = e.SummaryIndex
	}
	if index == nil {
		return ErrStream
	}
	channel := 1
	if summary {
		channel = 0
	}
	if partEvent {
		text, err := readPart(v["part"], "summary_text")
		if err != nil {
			return err
		}
		if b, ok := v["status"]; ok && !bytes.Equal(b, []byte("null")) {
			var status string
			if json.Unmarshal(b, &status) != nil || status != "incomplete" {
				return ErrStream
			}
			return failure(ErrResponse, provider.PhaseStream, provider.UpstreamIncomplete)
		}
		if e.Type == "response.reasoning_summary_part.done" {
			return p.hiddenText(s, *index, channel, text, true)
		}
		return p.hiddenSnapshot(s, *index, channel, text)
	}
	if e.Type == "response.reasoning_text.delta" || e.Type == "response.reasoning_summary_text.delta" {
		if e.Delta == nil {
			return ErrStream
		}
		return p.hiddenText(s, *index, channel, *e.Delta, false)
	}
	if e.Text == nil {
		return ErrStream
	}
	return p.hiddenText(s, *index, channel, *e.Text, true)
}

func (p *parser) collect(items outputItems) error {
	c := &completion{completionPayload: &completionPayload{}}
	var ordinary []string
	var spans uint32
	for index, item := range items {
		s := p.slots[index]
		c.projectionUnsupported = c.projectionUnsupported || s.projectionUnsupported
		switch item.Type {
		case "reasoning":
			if !s.reason.done || item.rawReason == nil {
				return ErrStream
			}
			r, e := readReasoning(item.rawReason.data)
			if e != nil {
				return e
			}
			c.handled = true
			if r.ep != continuation.Present || r.encrypted == "" {
				c.missingState = true
			}
			summary := make([]continuation.Part, 0, len(r.summary))
			for _, text := range r.summary {
				summary = append(summary, continuation.SummaryPart(text))
			}
			content := make([]continuation.Part, 0, len(r.content))
			for _, text := range r.content {
				content = append(content, continuation.ReasoningPart(text))
			}
			c.items = append(c.items, continuation.ReasoningItem(r.id, summary, content, r.cp, r.encrypted, r.ep, r.status, r.sp))
		case "message":
			refs := make([]uint32, 0, len(item.Content))
			for _, part := range item.Content {
				ordinary = append(ordinary, part.Text)
				refs = append(refs, spans)
				spans++
			}
			status, presence := "", continuation.Omitted
			if item.metadata != nil {
				status, presence = item.metadata.status, item.metadata.presence
				c.projectionUnsupported = c.projectionUnsupported || item.metadata.unsupported
			}
			phase, pp := "", continuation.Omitted
			if item.metadata != nil {
				phase, pp = item.metadata.phase, item.metadata.phasePresence
			}
			logprobs := make([]continuation.Presence, len(item.Content))
			for j, part := range item.Content {
				if part.metadata != nil {
					logprobs[j] = part.metadata.logprobsPresence
				}
			}
			c.items = append(c.items, continuation.MessageItemWithMetadata(item.ID, refs, status, presence, phase, pp, logprobs))
		case "function_call":
			status, presence := "", continuation.Omitted
			if item.metadata != nil {
				status, presence = item.metadata.status, item.metadata.presence
				c.projectionUnsupported = c.projectionUnsupported || item.metadata.unsupported
			}
			caller, async := continuation.Omitted, continuation.Omitted
			if item.metadata != nil {
				caller, async = item.metadata.callerPresence, item.metadata.asyncPresence
			}
			c.items = append(c.items, continuation.FunctionItemWithMetadata(item.ID, item.CallID, item.Name, s.args, status, presence, caller, async))
		}
	}
	capture, e := p.capture.CaptureLayout(ordinary)
	c.capture = capture
	c.projectionUnsupported = c.projectionUnsupported || e != nil
	if c.handled && p.calls == 0 && !capture.Meaningful() {
		return failure(ErrResponse, provider.PhaseStream, provider.ReasoningNoVisibleOutput)
	}
	p.completion = c
	return nil
}
