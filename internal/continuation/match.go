package continuation

import "github.com/Jghh42/atuin-ai/internal/atuin"

// latest selects from tool_use occurrences, NOT contiguous assistant groups.
// Every present reserved reference is parsed, including result-only tampering.
func latest(messages []atuin.Message) (SnapshotID, bool, error) {
	if len(messages) > atuin.MaxMessages {
		return newSnapshotID(snapshotIDPayload{}), false, failure(Limit)
	}
	blocks := 0
	for _, m := range messages {
		if len(m.Content) > atuin.MaxBlocks-blocks {
			return newSnapshotID(snapshotIDPayload{}), false, failure(Limit)
		}
		blocks += len(m.Content)
	}
	var last SnapshotID = newSnapshotID(snapshotIDPayload{})
	found := false
	for _, m := range messages {
		for _, b := range m.Content {
			id := ""
			if b.Type == "tool_use" {
				id = b.ID
			} else if b.Type == "tool_result" {
				id = b.ToolUseID
			}
			if !Reserved(id) {
				continue
			}
			p, e := parseHandle(id)
			if e != nil {
				return newSnapshotID(snapshotIDPayload{}), false, e
			}
			if b.Type == "tool_use" {
				last = p.snapshot
				found = true
			}
		}
	}
	// A reserved result without any reserved call must never fall back to legacy.
	if !found {
		for _, m := range messages {
			for _, b := range m.Content {
				if Reserved(b.ToolUseID) {
					return newSnapshotID(snapshotIDPayload{}), false, failure(BindingMismatch)
				}
			}
		}
	}
	return last, found, nil
}
func (k *keys) matchSegment(messages []atuin.Message, s Segment) (Match, error) {
	start := int(s.prefix.messages)
	out := newMatch(matchPayload{start: start})
	K := len(s.calls)
	if K == 0 || K > MaxCalls || start >= len(messages) {
		return newMatch(matchPayload{}), failure(BindingMismatch)
	}
	next := 0
	var calls, results uint32
	first := true
	previousAssistant := false
	for pos := start; pos < len(messages); pos++ {
		m := messages[pos]
		switch m.Role {
		case "assistant":
			if previousAssistant {
				return newMatch(matchPayload{}), failure(BindingMismatch)
			}
			previousAssistant = true
			offset := 0
			if first && s.text.bytes > 0 {
				if len(m.Content) == 0 || m.Content[0].Type != "text" {
					return newMatch(matchPayload{}), failure(BindingMismatch)
				}
				out.text = m.Content[0].Text
				expected, e := k.text(out.text)
				if e != nil || expected.bytes != s.text.bytes || !equal(expected.digest, s.text.digest) {
					return newMatch(matchPayload{}), failure(BindingMismatch)
				}
				offset = 1
			}
			if offset == len(m.Content) {
				return newMatch(matchPayload{}), failure(BindingMismatch)
			}
			for _, b := range m.Content[offset:] {
				if b.Type != "tool_use" || next >= K {
					return newMatch(matchPayload{}), failure(BindingMismatch)
				}
				c := s.calls[next]
				if b.ID != c.handle.value || b.Name != c.name {
					return newMatch(matchPayload{}), failure(BindingMismatch)
				}
				digest, e := k.arguments(b.Input, true)
				if e != nil || !equal(digest, c.publicArguments) {
					return newMatch(matchPayload{}), failure(BindingMismatch)
				}
				calls |= 1 << uint(next)
				next++
			}
			first = false
		case "user":
			previousAssistant = false
			if first || len(m.Content) != 1 || m.Content[0].Type != "tool_result" {
				return newMatch(matchPayload{}), failure(BindingMismatch)
			}
			b := m.Content[0]
			ordinal := -1
			for i, c := range s.calls {
				if c.handle.value == b.ToolUseID {
					ordinal = i
					break
				}
			}
			if ordinal < 0 || calls&(1<<uint(ordinal)) == 0 || results&(1<<uint(ordinal)) != 0 {
				return newMatch(matchPayload{}), failure(BindingMismatch)
			}
			results |= 1 << uint(ordinal)
			out.results = append(out.results, pos)
		default:
			return newMatch(matchPayload{}), failure(BindingMismatch)
		}
		mask := uint32((uint64(1) << uint(K)) - 1)
		if calls == mask && results == mask {
			out.end = pos + 1
			return out, nil
		}
	}
	return newMatch(matchPayload{}), failure(BindingMismatch)
}
func (k *keys) verifyRecord(r Record, messages []atuin.Message, scope Commitment) ([]Match, error) {
	if !equal(r.scope, scope) || len(r.segments) == 0 || len(r.segments) > MaxSegments {
		return nil, failure(BindingMismatch)
	}
	latestID, found, e := latest(messages)
	if e != nil {
		return nil, e
	}
	if !found || !sameSnapshot(latestID, r.snapshot) || !sameSnapshot(r.segments[len(r.segments)-1].snapshot, r.snapshot) {
		return nil, failure(BindingMismatch)
	}
	allowed := map[string]bool{}
	positions := map[string][2]int{}
	backends := map[string]bool{}
	snapshots := map[[16]byte]bool{}
	matches := makeMatch(0, len(r.segments))
	end := 0
	for _, s := range r.segments {
		if snapshots[s.snapshot.bytes] || int(s.prefix.messages) < end || int(s.prefix.messages) > len(messages) {
			return nil, failure(BindingMismatch)
		}
		snapshots[s.snapshot.bytes] = true
		prefix, e := k.prefix(messages[:int(s.prefix.messages)])
		if e != nil || !equal(prefix.digest, s.prefix.digest) {
			return nil, failure(BindingMismatch)
		}
		for ordinal, c := range s.calls {
			p, e := k.verify(c.handle, scope)
			if e != nil || !sameSnapshot(p.snapshot, s.snapshot) || int(p.ordinal) != ordinal || allowed[c.handle.value] || backends[c.backend.value] || c.backend.value == "" || Reserved(c.backend.value) {
				return nil, failure(BindingMismatch)
			}
			allowed[c.handle.value] = true
			backends[c.backend.value] = true
		}
		match, e := k.matchSegment(messages, s)
		if e != nil {
			return nil, e
		}
		// Check layout without retaining all reconstructed text after verification.
		parts, e := s.recipe.Restore(match.text)
		for _, p := range parts {
			clear(p)
		}
		if e != nil {
			return nil, e
		}
		for _, c := range s.calls {
			positions[c.handle.value] = [2]int{match.start, match.end}
		}
		matches = append(matches, match)
		end = match.end
	}
	// MAC/membership is not occurrence authority. Independently constrain EVERY
	// protected occurrence to its matched segment and exactly one use/result
	// across the ENTIRE history, including committed prefixes, gaps and suffixes.
	usedCalls, usedResults := map[string]bool{}, map[string]bool{}
	for pos, m := range messages {
		for _, b := range m.Content {
			id := ""
			if b.Type == "tool_use" {
				id = b.ID
			} else if b.Type == "tool_result" {
				id = b.ToolUseID
			}
			if Reserved(id) {
				if !allowed[id] {
					return nil, failure(BindingMismatch)
				}
				if _, e := k.verify(newPublicHandle(publicHandlePayload{value: id}), scope); e != nil {
					return nil, e
				}
				interval := positions[id]
				if pos < interval[0] || pos >= interval[1] {
					return nil, failure(BindingMismatch)
				}
				if b.Type == "tool_use" {
					if usedCalls[id] {
						return nil, failure(BindingMismatch)
					}
					usedCalls[id] = true
				} else {
					if usedResults[id] {
						return nil, failure(BindingMismatch)
					}
					usedResults[id] = true
				}
			}
			if b.Type == "tool_use" && !Reserved(id) && backends[id] {
				return nil, failure(BindingMismatch)
			}
		}
	}
	for id := range allowed {
		if !usedCalls[id] || !usedResults[id] {
			return nil, failure(BindingMismatch)
		}
	}
	return matches, nil
}
