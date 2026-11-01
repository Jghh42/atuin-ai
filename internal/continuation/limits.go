package continuation

import "unicode/utf8"

// One scalar policy is shared by structural validation and the explicit codec.
// These retain the v2 decoder ceilings; canonical public prefixes separately
// remain bounded by atuin.MaxMessages (4096). No cap is expanded for round trips.
const (
	recordMagic           = "aic-record2"
	maxRecordPayloadBytes = MaxRecordBytes - 160 // conservative envelope reservation
	maxNameBytes          = 256
	maxIDBytes            = MaxRecordBytes
	maxArgumentsBytes     = 256 << 10
	maxEncryptedBytes     = MaxRecordBytes
	maxPartTextBytes      = MaxRecordBytes
	maxStatusBytes        = 32
	maxPhaseBytes         = 16
	maxPrefixMessages     = 65536
	maxVisibleTextBytes   = 4 << 20
	maxRawTextBytes       = maxVisibleTextBytes + MaxLeadingBytes
	maxOutputItems        = 128
	maxItemKind           = 3
	maxPartKind           = 3
	maxPresence           = 2
)

// Check collection counts and scalar limits BEFORE any ownership clones. This
// is structural preflight, not authenticity; full contract validation remains
// mandatory. Fixed ceilings make all additions below overflow-safe.
func boundedInputs(ancestry []Segment, items []Item, calls []Call, recipe TextRecipe) error {
	if len(ancestry) >= MaxSegments || len(items) > maxOutputItems || len(calls) > MaxCalls {
		return failure(Limit)
	}
	totalItems, totalParts, totalSpans, bytes := 0, 0, 0, 0
	charge := func(n int) bool {
		if n > MaxRecordBytes-bytes {
			return false
		}
		bytes += n
		return true
	}
	scalar := func(s string, max int) error {
		if len(s) > max || !charge(len(s)) {
			return failure(Limit)
		}
		if !utf8.ValidString(s) {
			return failure(Corrupt)
		}
		return nil
	}
	visit := func(items []Item, calls []Call, recipe TextRecipe) error {
		if recipe.textRecipePayload == nil || len(calls) > MaxCalls {
			return failure(Corrupt)
		}
		if len(recipe.leading) > MaxLeadingBytes || recipe.rawBytes > maxRawTextBytes {
			return failure(Limit)
		}
		if !utf8.Valid(recipe.leading) {
			return failure(Corrupt)
		}
		if len(items) > maxOutputItems || len(items) > MaxItems-totalItems || len(recipe.spans) > MaxSpans-totalSpans {
			return failure(Limit)
		}
		totalItems += len(items)
		totalSpans += len(recipe.spans)
		if !charge(len(recipe.leading)) {
			return failure(Limit)
		}
		for _, span := range recipe.spans {
			if span.spanPayload == nil {
				return failure(Corrupt)
			}
			if span.bytes > maxRawTextBytes {
				return failure(Limit)
			}
		}
		for _, call := range calls {
			if call.callPayload == nil {
				return failure(Corrupt)
			}
			for _, v := range []struct {
				s   string
				max int
			}{{call.name, maxNameBytes}, {call.backend.Value(), maxIDBytes}, {call.handle.Value(), HandleBytes}} {
				if e := scalar(v.s, v.max); e != nil {
					return e
				}
			}
		}
		for _, item := range items {
			if item.itemPayload == nil {
				return failure(Corrupt)
			}
			if item.kind > maxItemKind || item.contentPresence > maxPresence || item.encryptedPresence > maxPresence || item.statusPresence > maxPresence || item.phasePresence > maxPresence || item.callerPresence > maxPresence || item.asyncPresence > maxPresence {
				return failure(Corrupt)
			}
			if len(item.summary) > MaxParts-totalParts {
				return failure(Limit)
			}
			totalParts += len(item.summary)
			if len(item.content) > MaxParts-totalParts {
				return failure(Limit)
			}
			totalParts += len(item.content)
			for _, v := range []struct {
				s   string
				max int
			}{{item.iD, maxIDBytes}, {item.callID, maxIDBytes}, {item.name, maxNameBytes}, {item.arguments, maxArgumentsBytes}, {item.encrypted, maxEncryptedBytes}, {item.status, maxStatusBytes}, {item.phase, maxPhaseBytes}} {
				if e := scalar(v.s, v.max); e != nil {
					return e
				}
			}
			for _, parts := range [][]Part{item.summary, item.content} {
				for _, part := range parts {
					if part.partPayload == nil || part.kind > maxPartKind || part.logprobsPresence > maxPresence {
						return failure(Corrupt)
					}
					if part.span > MaxSpans {
						return failure(Limit)
					}
					if e := scalar(part.text, maxPartTextBytes); e != nil {
						return e
					}
				}
			}
		}
		return nil
	}
	if e := visit(items, calls, recipe); e != nil {
		return e
	}
	for _, s := range ancestry {
		if s.segmentPayload == nil {
			return failure(Corrupt)
		}
		if e := visit(s.items, s.calls, s.recipe); e != nil {
			return e
		}
	}
	return nil
}
