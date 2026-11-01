package codex

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"hash"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/continuation"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

const maxOutputItems = 128

type outputItem struct {
	rawReason *rawReasoning
	metadata  *itemMetadata
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Namespace json.RawMessage `json:"namespace"`
	Content   outputContent   `json:"content"`
}

type outputText struct {
	metadata *itemMetadata
	Type     string `json:"type"`
	Text     string `json:"text"`
}
type outputContent []outputText

func (parts *outputContent) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		*parts = nil
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('[') {
		return ErrStream
	}
	out := make(outputContent, 0)
	for d.More() {
		if len(out) >= maxOutputItems {
			return ErrStream
		}
		var part outputText
		if d.Decode(&part) != nil {
			return ErrStream
		}
		out = append(out, part)
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim(']') {
		return ErrStream
	}
	*parts = out
	return nil
}

// A missing lifecycle output is allowed; an explicitly supplied output must
// be an array, not null or another JSON type. Completion requires the array.
type outputItems []outputItem

func (items *outputItems) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return ErrStream
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('[') {
		return ErrStream
	}
	out := make(outputItems, 0)
	for d.More() {
		if len(out) >= maxOutputItems {
			return ErrStream
		}
		var item outputItem
		if d.Decode(&item) != nil {
			return ErrStream
		}
		out = append(out, item)
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim(']') {
		return ErrStream
	}
	*items = out
	return nil
}

type wireEvent struct {
	Type         string      `json:"type"`
	Sequence     *int64      `json:"sequence_number"`
	Index        *int        `json:"output_index"`
	ContentIndex *int        `json:"content_index"`
	SummaryIndex *int        `json:"summary_index"`
	ItemID       string      `json:"item_id"`
	ResponseID   *string     `json:"response_id"`
	Delta        *string     `json:"delta"`
	Arguments    *string     `json:"arguments"`
	Text         *string     `json:"text"`
	Item         *outputItem `json:"item"`
	Response     *struct {
		ID     *string         `json:"id"`
		Status string          `json:"status"`
		Output outputItems     `json:"output"`
		Usage  json.RawMessage `json:"usage"`
	} `json:"response"`
}
type textState struct {
	sum            hash.Hash
	size           int64
	done           bool
	metadataDone   bool
	metadataDigest [32]byte
}
type outputSlot struct {
	reason                 *reasoningSlot
	item                   outputItem
	initial                [32]byte
	identity, callIdentity [32]byte
	args                   string
	argsDone               bool
	texts                  map[int]*textState
	metadataDone           bool
	metadataDigest         [32]byte
	projectionUnsupported  bool
}
type parser struct {
	privateValue
	*parserPayload
}
type parserPayload struct {
	prepared   bool
	capture    *continuation.TextCapture
	completion *completion
	slots      map[int]*outputSlot
	sink       func(provider.Event) error
	calls      int
	textCount  int
	sequence   int64
	sequenced  bool
	previous   [32]byte
	usage      provider.Usage
	responseID string
}

func parseStream(r io.Reader, sink func(provider.Event) error) (provider.Usage, error) {
	u, _, e := parseConfigured(r, sink, false)
	return u, e
}
func parseConfigured(r io.Reader, sink func(provider.Event) error, prepared bool) (provider.Usage, *completion, error) {
	var sinkErr error
	capture := continuation.NewTextCapture()
	defer capture.Close()
	p := parser{parserPayload: &parserPayload{slots: make(map[int]*outputSlot), prepared: prepared, capture: capture, sink: func(e provider.Event) error {
		if prepared && e.Kind == provider.TextDelta {
			_ = capture.Add(e.Text)
		}
		sinkErr = sink(e)
		return sinkErr
	}}}
	err := readSSE(r, p.event)
	if err != nil {
		if p.completion != nil {
			p.completion.close()
		}
		if sinkErr == nil {
			err = streamFailure(err)
		}
		return provider.Usage{}, nil, err
	}
	return p.usage, p.completion, nil
}
func (p *parser) event(name string, data []byte) (bool, error) {
	if bytes.Equal(data, []byte("[DONE]")) || !validStreamJSON(data, true) {
		return false, ErrStream
	}
	var e wireEvent
	if json.Unmarshal(data, &e) != nil || e.Type == "" || (name != "" && name != e.Type) {
		return false, ErrStream
	}
	defer func() {
		if e.Item != nil && e.Item.rawReason != nil {
			clear(e.Item.rawReason.data)
			e.Item.rawReason.data = nil
		}
		if e.Response != nil {
			for _, item := range e.Response.Output {
				if item.rawReason != nil {
					clear(item.rawReason.data)
					item.rawReason.data = nil
				}
			}
		}
	}()
	digest := sha256.Sum256(data)
	if e.Sequence != nil {
		if *e.Sequence < 0 {
			return false, ErrStream
		}
		if p.sequenced && *e.Sequence <= p.sequence {
			if *e.Sequence == p.sequence && digest == p.previous {
				return false, nil
			}
			return false, ErrStream
		}
		p.sequenced = true
		p.sequence = *e.Sequence
		p.previous = digest
	}
	if err := p.identity(e); err != nil {
		return false, err
	}
	switch e.Type {
	case "response.created", "response.in_progress", "response.queued":
		// Until lifecycle snapshots have full reconciliation support, any
		// nonempty output is unsupported, even with a matching response ID.
		// Never silently drop reasoning, execution, refusal, or other content.
		if len(e.Response.Output) != 0 {
			return false, p.unsupported(provider.UnsupportedLifecycleOutput)
		}
	case "error":
		return false, failure(ErrResponse, provider.PhaseStream, provider.UpstreamError)
	case "response.failed":
		return false, failure(ErrResponse, provider.PhaseStream, provider.UpstreamFailed)
	case "response.incomplete":
		return false, failure(ErrResponse, provider.PhaseStream, provider.UpstreamIncomplete)
	case "response.refusal.delta", "response.refusal.done":
		return false, ErrRefusal
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done", "response.reasoning_text.delta", "response.reasoning_text.done":
		return false, p.reasonEvent(e, data)
	case "response.output_item.added", "response.output_item.done":
		if p.prepared && e.Item != nil && e.Item.Type == "reasoning" {
			if _, err := strictObject(data, "type", "sequence_number", "output_index", "item", "response_id"); err != nil || e.Sequence == nil {
				return false, ErrStream
			}
		}
		if e.Index == nil || e.Item == nil {
			return false, ErrStream
		}
		_, err := p.item(*e.Index, *e.Item, e.Type == "response.output_item.done")
		return false, err
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		s, err := p.slot(e, "function_call")
		if err != nil {
			return false, err
		}
		if e.Type == "response.function_call_arguments.delta" {
			if e.Delta == nil || s.argsDone || len(s.args)+len(*e.Delta) > config.MaxCallArgumentsBytes {
				return false, ErrStream
			}
			s.args += *e.Delta
		} else {
			if e.Arguments == nil {
				return false, ErrStream
			}
			if err := s.snapshot(*e.Arguments); err != nil {
				return false, err
			}
		}
	case "response.output_text.delta", "response.output_text.done":
		s, err := p.slot(e, "message")
		if err != nil {
			return false, err
		}
		if e.ContentIndex == nil {
			return false, ErrStream
		}
		if e.Type == "response.output_text.delta" {
			if e.Delta == nil {
				return false, ErrStream
			}
			return false, p.text(s, *e.ContentIndex, *e.Delta, false)
		}
		if e.Text == nil {
			return false, ErrStream
		}
		return false, p.text(s, *e.ContentIndex, *e.Text, true)
	case "response.content_part.added", "response.content_part.done":
		// Inspect content parts, including refusal, rather than ignoring an output
		// channel that could carry unsupported state.
		var part struct {
			Part outputText `json:"part"`
		}
		if json.Unmarshal(data, &part) != nil {
			return false, ErrStream
		}
		if part.Part.Type == "refusal" {
			return false, ErrRefusal
		}
		if part.Part.Type != "output_text" {
			return false, p.unsupported(provider.UnsupportedContent)
		}
		s, err := p.slot(e, "message")
		if err != nil || e.ContentIndex == nil {
			return false, ErrStream
		}
		if err := p.partMetadata(s, *e.ContentIndex, part.Part.metadata, e.Type == "response.content_part.done"); err != nil {
			return false, err
		}
		if e.Type == "response.content_part.done" {
			return false, p.text(s, *e.ContentIndex, part.Part.Text, true)
		}
		// Only empty initial parts are supported; text arrives via deltas or
		// final snapshots, avoiding an ambiguous duplicate prefix.
		if part.Part.Text != "" {
			return false, ErrStream
		}
		_, err = p.textState(s, *e.ContentIndex)
		return false, err
	case "response.completed":
		if e.Response == nil || e.Response.Status != "completed" || e.Response.Output == nil || len(e.Response.Output) > maxOutputItems {
			return false, ErrStream
		}
		// Cardinality cannot detect omitted sparse indices. Check every
		// observed output AND content slot before processing final snapshots.
		for index, slot := range p.slots {
			if index >= len(e.Response.Output) {
				return false, ErrStream
			}
			if slot.item.Type == "message" {
				if !contentBounds(slot, len(e.Response.Output[index].Content)) {
					return false, ErrStream
				}
			}
		}
		for i, item := range e.Response.Output {
			if _, err := p.item(i, item, true); err != nil {
				return false, err
			}
		}
		usage, err := parseUsage(e.Response.Usage)
		if err != nil {
			return false, err
		}
		var calls []tools.Call
		for i := range e.Response.Output {
			s := p.slots[i]
			if s.item.Type == "function_call" {
				if !s.argsDone || !validArguments(s.args) {
					return false, ErrStream
				}
				calls = append(calls, tools.Call{ID: s.item.CallID, Name: s.item.Name, Input: json.RawMessage(s.args)})
			}
		}
		if p.prepared {
			if err := p.collect(e.Response.Output); err != nil {
				return false, err
			}
		}
		if len(calls) > 0 {
			if err := p.sink(provider.Event{Kind: provider.ToolCalls, Calls: calls}); err != nil {
				return false, err
			}
		}
		p.usage = usage
		return true, nil // completion is terminal; close rather than wait for EOF
	default:
		// Unknown additive metadata is ignored, but unknown execution/reasoning
		// channels fail closed even if an eventual output snapshot omits them.
		if unsupportedChannel(e.Type) || p.prepared && unsupportedMediaChannel(e.Type) {
			return false, p.unsupported(provider.UnsupportedChannel)
		}
	}
	return false, nil
}

func (p *parser) unsupported(category provider.Category) error {
	if !p.prepared {
		return ErrUnsupported
	}
	return failure(ErrUnsupported, provider.PhaseStream, category)
}
func unsupportedMediaChannel(kind string) bool {
	channel := strings.TrimPrefix(kind, "response.")
	for _, family := range []string{"audio", "output_audio", "video", "output_video", "image"} {
		if channel == family || strings.HasPrefix(channel, family+"_") || strings.HasPrefix(channel, family+".") {
			return true
		}
	}
	return false
}
func unsupportedChannel(kind string) bool {
	for _, marker := range []string{"reasoning", "tool", "_call", "refusal", "output_item", "content_part"} {
		if strings.Contains(kind, marker) {
			return true
		}
	}
	// Associated execution/approval channels may not have a *_call spelling.
	// Reject the whole unsupported family, not just its final output items.
	channel := strings.TrimPrefix(kind, "response.")
	for _, family := range []string{"web_search", "file_search", "code_interpreter", "image_generation", "computer", "local_shell", "shell", "apply_patch", "mcp", "custom"} {
		if channel == family || strings.HasPrefix(channel, family+"_") || strings.HasPrefix(channel, family+".") {
			return true
		}
	}
	return false
}

// Lifecycle snapshots require explicit identity, including terminal-only
// completion. Other events may omit identity, but any supplied response ID
// must be valid and agree with every earlier observation. Never invent IDs.
func (p *parser) identity(e wireEvent) error {
	switch e.Type {
	case "response.created", "response.in_progress", "response.queued", "response.completed":
		if e.Response == nil || e.Response.ID == nil {
			return ErrStream
		}
	}
	ids := []*string{e.ResponseID}
	if e.Response != nil {
		ids = append(ids, e.Response.ID)
	}
	for _, id := range ids {
		if id == nil {
			continue
		}
		if !identifier(*id) || (p.responseID != "" && p.responseID != *id) {
			return ErrStream
		}
		p.responseID = *id
	}
	return nil
}

func (p *parser) slot(e wireEvent, kind string) (*outputSlot, error) {
	if e.Index == nil {
		return nil, ErrStream
	}
	s := p.slots[*e.Index]
	if s == nil || s.item.Type != kind || e.ItemID == "" || slotID(s) != e.ItemID {
		return nil, ErrStream
	}
	return s, nil
}
func (p *parser) item(index int, item outputItem, final bool) (*outputSlot, error) {
	if item.Type == "reasoning" {
		return p.reasonItem(index, item, final)
	}
	if index < 0 || index >= maxOutputItems || !identifier(item.ID) {
		return nil, ErrStream
	}
	if item.Type != "message" && item.Type != "function_call" {
		return nil, p.unsupported(provider.UnsupportedItem)
	}
	if len(item.Namespace) > 0 && string(item.Namespace) != "null" {
		return nil, p.unsupported(provider.UnsupportedNamespace)
	}
	if item.metadata != nil && item.metadata.executionInvalid {
		return nil, p.unsupported(provider.UnsupportedChannel)
	}
	if item.metadata != nil && item.metadata.presence == continuation.Present && (item.metadata.status == "incomplete" || final && item.metadata.status != "completed") {
		if item.metadata.status == "incomplete" {
			return nil, failure(ErrResponse, provider.PhaseStream, provider.UpstreamIncomplete)
		}
		return nil, ErrStream
	}
	if item.Type == "function_call" && (continuation.Reserved(item.CallID) || !identifier(item.CallID) || !functionName(item.Name)) {
		return nil, ErrStream
	}
	s := p.slots[index]
	if s == nil {
		for _, other := range p.slots {
			if slotID(other) == item.ID || (item.Type == "function_call" && other.callIdentity == ordinaryCallIdentity(item.CallID)) {
				return nil, ErrStream
			}
		}
		if item.Type == "function_call" {
			p.calls++
			if p.calls > config.MaxCallsPerResponse {
				return nil, ErrStream
			}
		}
		if len(item.Arguments) > config.MaxCallArgumentsBytes {
			return nil, ErrStream
		}
		// Construct a MINIMAL per-kind checkpoint. Inapplicable message fields
		// may fill a frame but must never become retained strings/slices. Keep
		// their legacy identity/retransmission comparisons via fixed digests.
		checkpoint := outputItem{Type: "message", ID: strings.Clone(item.ID)}
		args := ""
		if item.Type == "function_call" {
			checkpoint.Type = "function_call"
			checkpoint.CallID = strings.Clone(item.CallID)
			checkpoint.Name = strings.Clone(item.Name)
			args = strings.Clone(item.Arguments)
		}
		s = &outputSlot{item: checkpoint, identity: ordinaryIdentity(item), callIdentity: ordinaryCallIdentity(item.CallID), initial: itemDigest(item, p.prepared), args: args, texts: make(map[int]*textState)}
		p.slots[index] = s
	} else {
		if ordinaryIdentity(item) != s.identity {
			return nil, ErrStream
		}
		if !final {
			if itemDigest(item, p.prepared) != s.initial {
				return nil, ErrStream
			}
			return s, nil
		}
	}
	if item.metadata != nil {
		s.projectionUnsupported = s.projectionUnsupported || item.metadata.sticky || final && item.metadata.unsupported
		if p.prepared && final {
			if s.metadataDone && s.metadataDigest != item.metadata.digest {
				return nil, ErrStream
			}
			s.metadataDone = true
			s.metadataDigest = item.metadata.digest
		}
	}
	if item.Type == "function_call" {
		if final {
			if err := s.snapshot(item.Arguments); err != nil {
				return nil, err
			}
		}
	} else {
		if len(item.Content) > maxOutputItems {
			return nil, ErrStream
		}
		if final && !contentBounds(s, len(item.Content)) {
			return nil, ErrStream
		}
		for i, part := range item.Content {
			if part.Type == "refusal" {
				return nil, ErrRefusal
			}
			if part.Type != "output_text" {
				return nil, p.unsupported(provider.UnsupportedContent)
			}
			if err := p.partMetadata(s, i, part.metadata, final); err != nil {
				return nil, err
			}
			// added snapshots are normally empty; any text is a prefix, not final.
			if err := p.text(s, i, part.Text, final); err != nil {
				return nil, err
			}
		}
	}
	return s, nil
}
func ordinaryIdentity(item outputItem) [32]byte {
	h := sha256.New()
	digestField(h, "ordinary-slot-identity1")
	for _, v := range []string{item.Type, item.ID, item.CallID, item.Name} {
		digestField(h, v)
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
func ordinaryCallIdentity(call string) [32]byte {
	h := sha256.New()
	digestField(h, "ordinary-slot-callid1")
	digestField(h, call)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
func itemDigest(item outputItem, prepared bool) [32]byte {
	// Same decoded-field equality as the legacy JSON snapshot, without a
	// frame-sized json.Marshal scratch buffer entering stdlib encoder pools.
	// Namespace is already proven absent/null; raw/metadata pointers were never
	// JSON fields. Preserve nil-vs-empty content and exact ordered part bytes.
	h := sha256.New()
	digestField(h, "ordinary-initial-decoded1")
	for _, v := range []string{item.Type, item.ID, item.CallID, item.Name, item.Arguments} {
		digestField(h, v)
	}
	if item.Content == nil {
		digestField(h, "null")
	} else {
		digestField(h, "array")
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(item.Content)))
	_, _ = h.Write(count[:])
	for _, part := range item.Content {
		digestField(h, part.Type)
		digestField(h, part.Text)
	}
	if prepared && item.metadata != nil {
		_, _ = h.Write(item.metadata.digest[:])
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
func (p *parser) partMetadata(s *outputSlot, index int, m *itemMetadata, final bool) error {
	if !p.prepared || m == nil {
		return nil
	}
	state, e := p.textState(s, index)
	if e != nil {
		return e
	}
	s.projectionUnsupported = s.projectionUnsupported || m.sticky || final && m.unsupported
	if final {
		if state.metadataDone && state.metadataDigest != m.digest {
			return ErrStream
		}
		state.metadataDone = true
		state.metadataDigest = m.digest
	}
	return nil
}
func (s *outputSlot) snapshot(args string) error {
	if len(args) > config.MaxCallArgumentsBytes || (s.argsDone && args != s.args) || !strings.HasPrefix(args, s.args) || !validArguments(args) {
		return ErrStream
	}
	s.args = strings.Clone(args)
	s.argsDone = true
	return nil
}
func validArguments(s string) bool {
	return validStreamJSON([]byte(s), false) && strings.HasPrefix(strings.TrimSpace(s), "{")
}
func identifier(s string) bool {
	if len(s) == 0 || len(s) > 256 {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func functionName(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func (p *parser) textState(s *outputSlot, index int) (*textState, error) {
	if index < 0 || index >= maxOutputItems {
		return nil, ErrStream
	}
	state := s.texts[index]
	if state == nil {
		if p.textCount >= maxOutputItems {
			return nil, ErrStream
		}
		p.textCount++
		state = &textState{sum: sha256.New()}
		s.texts[index] = state
	}
	return state, nil
}

func contentBounds(s *outputSlot, count int) bool {
	for index := range s.texts {
		if index < 0 || index >= count {
			return false
		}
	}
	return true
}

func (p *parser) text(s *outputSlot, index int, text string, final bool) error {
	if !utf8.ValidString(text) {
		return ErrStream
	}
	state, err := p.textState(s, index)
	if err != nil {
		return err
	}
	if final {
		if state.size == 0 && !state.done && text != "" {
			if err := p.sink(provider.Event{Kind: provider.TextDelta, Text: text}); err != nil {
				return err
			}
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
	if text != "" {
		if err := p.sink(provider.Event{Kind: provider.TextDelta, Text: text}); err != nil {
			return err
		}
	}
	_, _ = state.sum.Write([]byte(text))
	state.size += int64(len(text))
	return nil
}

func parseUsage(raw json.RawMessage) (provider.Usage, error) {
	var u provider.Usage
	if len(raw) == 0 || string(raw) == "null" {
		return u, nil
	}
	var wire struct {
		Input   *int64 `json:"input_tokens"`
		Output  *int64 `json:"output_tokens"`
		Total   *int64 `json:"total_tokens"`
		Details *struct {
			Cached *int64 `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	}
	if !validStreamJSON(raw, true) || json.Unmarshal(raw, &wire) != nil {
		return u, ErrStream
	}
	set := func(p *int64, dst *int64, known *bool) bool {
		if p == nil {
			return true
		}
		if *p < 0 {
			return false
		}
		*dst = *p
		*known = true
		return true
	}
	if !set(wire.Input, &u.InputTokens, &u.InputKnown) || !set(wire.Output, &u.OutputTokens, &u.OutputKnown) || !set(wire.Total, &u.TotalTokens, &u.TotalKnown) {
		return u, ErrStream
	}
	if wire.Details != nil && !set(wire.Details.Cached, &u.CachedTokens, &u.CachedKnown) {
		return u, ErrStream
	}
	if u.CachedKnown && u.InputKnown && u.CachedTokens > u.InputTokens {
		return u, ErrStream
	}
	if u.TotalKnown && ((u.InputKnown && u.InputTokens > u.TotalTokens) || (u.OutputKnown && u.OutputTokens > u.TotalTokens) || (u.CachedKnown && u.CachedTokens > u.TotalTokens)) {
		return u, ErrStream
	}
	if u.InputKnown && u.OutputKnown {
		if u.InputTokens > math.MaxInt64-u.OutputTokens {
			return u, ErrStream
		}
		total := u.InputTokens + u.OutputTokens
		if u.TotalKnown && u.TotalTokens != total {
			return u, ErrStream
		}
		if !u.TotalKnown {
			u.TotalTokens = total
			u.TotalKnown = true
		}
	}
	return u, nil
}
