package continuation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

// Presence distinguishes output omission/null/value without exposing wire data.
type Presence uint8

const (
	Omitted Presence = iota
	Null
	Present
)

func SummaryPart(text string) Part   { return newPart(partPayload{kind: 2, text: text}) }
func ReasoningPart(text string) Part { return newPart(partPayload{kind: 3, text: text}) }
func ReasoningItem(id string, summary, content []Part, cp Presence, encrypted string, ep Presence, status string, sp Presence) Item {
	return ownedItem(itemPayload{kind: 3, iD: id, summary: summary, content: content, contentPresence: uint8(cp), encrypted: encrypted, encryptedPresence: uint8(ep), status: status, statusPresence: uint8(sp)})
}
func FunctionItem(id, callID, name, args, status string, sp Presence) Item {
	return ownedItem(itemPayload{kind: 2, iD: id, callID: callID, name: name, arguments: args, status: status, statusPresence: uint8(sp)})
}
func FunctionItemWithMetadata(id, callID, name, args, status string, sp, caller, async Presence) Item {
	return ownedItem(itemPayload{kind: 2, iD: id, callID: callID, name: name, arguments: args, status: status, statusPresence: uint8(sp), callerPresence: uint8(caller), asyncPresence: uint8(async)})
}
func MessageItem(id string, spans []uint32, status string, presence Presence) Item {
	return MessageItemWithMetadata(id, spans, status, presence, "", Omitted, nil)
}
func MessageItemWithMetadata(id string, spans []uint32, status string, presence Presence, phase string, pp Presence, logprobs []Presence) Item {
	if len(logprobs) != 0 && len(logprobs) != len(spans) {
		return Item{}
	}
	if len(spans) > maxOutputItems {
		return Item{}
	}
	parts := makePart(len(spans))
	for j, n := range spans {
		parts[j] = newPart(partPayload{kind: 1, span: n})
		if len(logprobs) > 0 {
			parts[j].logprobsPresence = uint8(logprobs[j])
		}
	}
	return ownedItem(itemPayload{kind: 1, iD: id, contentPresence: 2, content: parts, status: status, statusPresence: uint8(presence), phase: phase, phasePresence: uint8(pp)})
}
func ownedItem(p itemPayload) Item {
	borrowed := Item{itemPayload: &p}
	if boundedInputs(nil, []Item{borrowed}, nil, newTextRecipe(textRecipePayload{})) != nil {
		return Item{}
	}
	return newItem(p)
}
func ExecutableCall(id, name string) Call {
	return newCall(callPayload{backend: NewBackendCallID(id), name: name})
}

func NewStoreWithClock(dir *credentials.ContinuationDirectory, now func() time.Time) (*Store, error) {
	s, e := NewStore(dir)
	if e == nil && now != nil {
		s.now = now
	}
	return s, e
}
func Marked(messages []atuin.Message) (bool, error) {
	_, marked, e := latest(messages)
	return marked, e
}
func (r Record) Empty() bool { return r.recordPayload == nil || len(r.segments) == 0 }
func (r Record) BackendIDs() []string {
	var out []string
	if r.Empty() {
		return out
	}
	for _, s := range r.segments {
		for _, c := range s.calls {
			out = append(out, c.backend.Value())
		}
	}
	return out
}
func (r Record) NewHandles() []string {
	if r.Empty() {
		return nil
	}
	calls := r.segments[len(r.segments)-1].calls
	out := make([]string, len(calls))
	for j, c := range calls {
		out[j] = c.handle.Value()
	}
	return out
}
func (m Match) Bounds() (int, int) {
	if m.matchPayload == nil {
		return 0, 0
	}
	return m.start, m.end
}
func (l *Lease) BuildNext(scope Commitment, prefix Prefix, text TextExpectation, recipe TextRecipe, calls []Call, items []Item, prior Record) (Record, error) {
	var ancestry []Segment
	if !prior.Empty() {
		ancestry = prior.segments
	}
	return l.Build(scope, prefix, text, recipe, calls, items, ancestry)
}

// ReplayItems is a deliberate provider-input codec, NEVER automatic JSON on an
// opaque value. Call only with the matches returned by authenticated Load.
// Ordinary text and actual results are request-owned reconstruction, not saved.
func (r Record) ReplayItems(segment int, match Match, messages []atuin.Message) ([]json.RawMessage, error) {
	if r.Empty() || segment < 0 || segment >= len(r.segments) || match.matchPayload == nil {
		return nil, failure(Corrupt)
	}
	s := r.segments[segment]
	if match.start != int(s.prefix.messages) || match.end > len(messages) || len(match.results) != len(s.calls) {
		return nil, failure(BindingMismatch)
	}
	rawParts, e := s.recipe.Restore(match.text)
	if e != nil {
		return nil, e
	}
	defer func() {
		for _, b := range rawParts {
			clear(b)
		}
	}()
	out := make([]json.RawMessage, 0, len(s.items)+len(s.calls))
	encode := func(v any) error {
		b, e := json.Marshal(v)
		if e != nil {
			return failure(Corrupt)
		}
		out = append(out, b)
		return nil
	}
	for _, item := range s.items {
		v := map[string]any{"id": item.iD}
		switch item.kind {
		case 1:
			v["type"] = "message"
			v["role"] = "assistant"
			parts := make([]any, 0, len(item.content))
			for _, p := range item.content {
				if int(p.span) >= len(rawParts) {
					return nil, failure(Corrupt)
				}
				part := map[string]any{"type": "output_text", "text": string(rawParts[p.span]), "annotations": []any{}}
				if p.logprobsPresence == 2 {
					part["logprobs"] = []any{}
				}
				parts = append(parts, part)
			}
			v["content"] = parts
			if item.phasePresence == 1 {
				v["phase"] = nil
			} else if item.phasePresence == 2 {
				v["phase"] = item.phase
			}
		case 2:
			v["type"] = "function_call"
			v["call_id"] = item.callID
			v["name"] = item.name
			v["arguments"] = item.arguments
			if item.callerPresence == 1 {
				v["caller"] = nil
			} else if item.callerPresence == 2 {
				v["caller"] = map[string]any{"type": "direct"}
			}
			if item.asyncPresence == 2 {
				v["async"] = false
			}
		case 3:
			v["type"] = "reasoning"
			summary := make([]any, 0, len(item.summary))
			for _, p := range item.summary {
				summary = append(summary, map[string]any{"type": "summary_text", "text": p.text})
			}
			v["summary"] = summary
			if item.contentPresence == 2 {
				content := make([]any, 0, len(item.content))
				for _, p := range item.content {
					content = append(content, map[string]any{"type": "reasoning_text", "text": p.text})
				}
				v["content"] = content
			}
			if item.encryptedPresence == 1 {
				v["encrypted_content"] = nil
			} else if item.encryptedPresence == 2 {
				v["encrypted_content"] = item.encrypted
			}
		default:
			return nil, failure(Corrupt)
		}
		if item.statusPresence == 2 {
			v["status"] = item.status
		}
		if e := encode(v); e != nil {
			return nil, e
		}
	}
	for _, pos := range match.results {
		if pos < match.start || pos >= match.end || len(messages[pos].Content) != 1 {
			return nil, failure(BindingMismatch)
		}
		b := messages[pos].Content[0]
		backend := ""
		for _, c := range s.calls {
			if c.handle.Value() == b.ToolUseID {
				backend = c.backend.Value()
				break
			}
		}
		if b.Type != "tool_result" || backend == "" {
			return nil, failure(BindingMismatch)
		}
		output := b.Content
		if b.IsError {
			output = "Tool execution failed: " + output
		}
		if e := encode(map[string]any{"type": "function_call_output", "call_id": backend, "output": output}); e != nil {
			return nil, e
		}
	}
	return out, nil
}

// PublishWithoutRecord still guards all direct sources and inherited expiry
// under the credential identity lease; text/suggestions never create an index.
func (l *Lease) PublishWithoutRecord(ctx context.Context, p credentials.PublicationLease, prior Record, emit func(context.Context) error) error {
	if emit == nil || p.Check() != nil || l.Check() != nil || ctx.Err() != nil {
		return failure(LifecycleChanged)
	}
	guard := func() error {
		if l.Check() != nil || p.Check() != nil || ctx.Err() != nil {
			return failure(LifecycleChanged)
		}
		if !prior.Empty() {
			now, e := l.store.nowFloor()
			if e != nil {
				return e
			}
			return checkExpiry(prior, now)
		}
		return nil
	}
	linked, cancel := context.WithCancel(l.Context())
	stop := context.AfterFunc(p.Context(), cancel)
	defer func() { stop(); cancel() }()
	original := newEmitContext(emitContextPayload{context: linked, source: ctx, scope: l.Context(), publication: p.Context()})
	if e := guard(); e != nil {
		return e
	}
	if original.Err() != nil {
		return failure(LifecycleChanged)
	}
	return emit(original)
}

// Captured contains only bounded private leading bytes and layout/hash metadata.
// It never retains ordinary visible text or per-delta spans.
type Captured struct {
	private
	*capturedPayload
}
type capturedPayload struct {
	recipe       TextRecipe
	clientBytes  uint32
	clientDigest [32]byte
	meaningful   bool
}

func (*Captured) LogValue() slog.Value { return slog.StringValue("[private continuation]") }
func (c Captured) Meaningful() bool    { return c.capturedPayload != nil && c.meaningful }
func (c *Captured) Clear() {
	if c == nil || c.capturedPayload == nil {
		return
	}
	clear(c.recipe.leading)
	*c = Captured{}
}
func (l *Lease) AcceptCapture(c Captured) (TextExpectation, TextRecipe, error) {
	s := l.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.checkLocked() != nil || s.key == nil || c.capturedPayload == nil {
		return TextExpectation{}, TextRecipe{}, failure(LifecycleChanged)
	}
	return s.key.textDigest(c.clientBytes, c.clientDigest), cloneTextRecipe(c.recipe), nil
}
