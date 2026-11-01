package codex

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"hash"
	"sort"
	"strconv"
	"strings"

	"github.com/Jghh42/atuin-ai/internal/continuation"
)

// Only finite supported metadata survives as bounded scalars/presence/digests.
// Raw annotation/caller/extension objects are NEVER retained in checkpoints.
func metadataDigest(raw map[string]json.RawMessage, exclude ...string) [32]byte {
	fields := make(map[string]any, len(raw))
	for k, b := range raw {
		skip := false
		for _, x := range exclude {
			skip = skip || k == x
		}
		if skip {
			continue
		}
		var value any
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		if d.Decode(&value) != nil {
			return sha256.Sum256(b)
		}
		fields[k] = value
	}
	// Exclude payload BYTES only, never their required presence/type. Empty
	// string/array is a value, distinct from missing, null or a wrong type.
	shapes := make([]uint8, 0, len(exclude))
	for _, key := range exclude {
		shapes = append(shapes, payloadShape(raw, key))
	}
	h := sha256.New()
	digestField(h, "metadata-json-shape1")
	hashMetadataValue(h, fields)
	digestField(h, string(shapes))
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// Canonical decoded-value hashing avoids retaining a frame-sized JSON encoder
// scratch buffer in stdlib pools. Depth/work are prebounded by validStreamJSON;
// arrays/order, decoded strings, exact numbers and sorted object keys survive.
func hashMetadataValue(h hash.Hash, value any) {
	switch v := value.(type) {
	case nil:
		digestField(h, "null")
	case bool:
		if v {
			digestField(h, "true")
		} else {
			digestField(h, "false")
		}
	case string:
		digestField(h, "string")
		digestField(h, v)
	case json.Number:
		digestField(h, "number")
		digestField(h, string(v))
	case []any:
		digestField(h, "array")
		digestField(h, strconv.Itoa(len(v)))
		for _, element := range v {
			hashMetadataValue(h, element)
		}
		digestField(h, "end-array")
	case map[string]any:
		digestField(h, "object")
		digestField(h, strconv.Itoa(len(v)))
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			digestField(h, key)
			hashMetadataValue(h, v[key])
		}
		digestField(h, "end-object")
	}
}
func payloadShape(raw map[string]json.RawMessage, key string) uint8 {
	b, ok := raw[key]
	if !ok {
		return 0
	}
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		return 1
	}
	if len(b) == 0 {
		return 7
	}
	switch b[0] {
	case '"':
		return 2
	case '[':
		return 3
	case '{':
		return 4
	case 't', 'f':
		return 5
	default:
		return 6
	}
}
func presence(raw map[string]json.RawMessage, key string) continuation.Presence {
	b, ok := raw[key]
	if !ok {
		return continuation.Omitted
	}
	if bytes.Equal(b, []byte("null")) {
		return continuation.Null
	}
	return continuation.Present
}
func emptyArray(b []byte) bool {
	b = bytes.TrimSpace(b)
	return len(b) >= 2 && b[0] == '[' && b[len(b)-1] == ']' && len(bytes.TrimSpace(b[1:len(b)-1])) == 0
}

// requiredMetadata separates permitted provisional omission from a supplied
// malformed nonnullable field. Call only for field-specific required predicates;
// optional nullable fields (phase/logprobs/function status/caller/async) do not
// pass through this classifier. DONE/final additionally authenticate omissions.
func (m *itemMetadata) requiredMetadata(raw map[string]json.RawMessage, key string, valid bool) {
	if !valid {
		m.unsupported = true
		m.sticky = m.sticky || presence(raw, key) != continuation.Omitted
	}
}

func textMetadata(raw map[string]json.RawMessage) *itemMetadata {
	m := &itemMetadata{itemMetadataPayload: &itemMetadataPayload{digest: metadataDigest(raw, "text")}}
	m.requiredMetadata(raw, "annotations", emptyArray(raw["annotations"]))
	_, textError := requiredString(raw, "text")
	m.requiredMetadata(raw, "text", textError == nil)
	m.logprobsPresence = presence(raw, "logprobs")
	if m.logprobsPresence == continuation.Present && !emptyArray(raw["logprobs"]) {
		m.unsupported = true
		m.sticky = true
	}
	for k := range raw {
		if k != "type" && k != "text" && k != "annotations" && k != "logprobs" {
			m.unsupported = true
			m.sticky = true
		}
	}
	return m
}
func outputMetadata(raw map[string]json.RawMessage, kind string) *itemMetadata {
	m := &itemMetadata{itemMetadataPayload: &itemMetadataPayload{digest: metadataDigest(raw, "arguments", "content")}}
	var e error
	m.status, m.presence, e = optionalString(raw, "status")
	m.unsupported = m.unsupported || e != nil || m.presence == continuation.Present && m.status != "completed"
	m.sticky = m.sticky || e != nil || m.presence == continuation.Present && m.status != "completed" && m.status != "in_progress" && m.status != "incomplete"
	if len(m.status) > 32 {
		m.status = ""
		m.unsupported = true
		m.sticky = true
	}
	if kind == "message" {
		role, err := requiredString(raw, "role")
		m.requiredMetadata(raw, "role", err == nil && role == "assistant")
		m.requiredMetadata(raw, "status", e == nil && m.presence == continuation.Present && (m.status == "completed" || m.status == "in_progress" || m.status == "incomplete"))
		m.phase, m.phasePresence, e = optionalString(raw, "phase")
		if e != nil || m.phasePresence == continuation.Present && m.phase != "commentary" && m.phase != "final_answer" {
			m.unsupported = true
			m.sticky = true
			m.phase = ""
		}
		// Hash content metadata separately; public text digest reconciliation remains
		// independent, and no complete per-DONE content/text snapshot is retained.
		var parts outputContent
		contentError := json.Unmarshal(raw["content"], &parts)
		m.requiredMetadata(raw, "content", payloadShape(raw, "content") == 3 && contentError == nil)
		digests := make([][32]byte, 0, len(parts))
		for _, part := range parts {
			if part.metadata != nil {
				digests = append(digests, part.metadata.digest)
				m.unsupported = m.unsupported || part.metadata.unsupported
				m.sticky = m.sticky || part.metadata.sticky
			}
		}
		encoded, _ := json.Marshal(struct {
			Header [32]byte
			Parts  [][32]byte
		}{m.digest, digests})
		m.digest = sha256.Sum256(encoded)
	} else if kind == "function_call" {
		_, argumentsError := requiredString(raw, "arguments")
		m.requiredMetadata(raw, "arguments", argumentsError == nil)
		m.callerPresence = presence(raw, "caller")
		if m.callerPresence == continuation.Present {
			v, err := strictObject(raw["caller"], "type")
			if err != nil {
				m.executionInvalid = true
			} else {
				typ, err := requiredString(v, "type")
				m.executionInvalid = m.executionInvalid || err != nil || typ != "direct"
			}
		}
		m.asyncPresence = presence(raw, "async")
		if m.asyncPresence == continuation.Present {
			var value bool
			if json.Unmarshal(raw["async"], &value) != nil || value {
				m.executionInvalid = true
			}
		}
	}
	for k := range raw {
		if kind == "function_call" && (strings.EqualFold(k, "caller") && k != "caller" || strings.EqualFold(k, "async") && k != "async") {
			m.executionInvalid = true
		}
		allowed := k == "id" || k == "type" || k == "status" || kind == "message" && (k == "role" || k == "content" || k == "phase") || kind == "function_call" && (k == "call_id" || k == "name" || k == "arguments" || k == "caller" || k == "async" || k == "namespace")
		if !allowed {
			m.unsupported = true
			m.sticky = true
		}
	}
	return m
}
func (part *outputText) UnmarshalJSON(raw []byte) error {
	type plain outputText
	var decoded plain
	if json.Unmarshal(raw, &decoded) != nil {
		return ErrStream
	}
	*part = outputText(decoded)
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ErrStream
	}
	part.metadata = textMetadata(fields)
	return nil
}
