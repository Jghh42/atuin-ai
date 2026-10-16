package atuin

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// Decoder's optional clock and randomness are injectable for deterministic tests.
// A Decoder may be shared if its injected functions/readers are concurrency safe.
type Decoder struct {
	Catalog ModelCatalog
	Now     func() time.Time
	Random  io.Reader
}

const (
	MaxJSONDepth     = 64
	MaxJSONStructure = 65_536
	MaxMessages      = 4096
	MaxBlocks        = 16_384
)

func (d Decoder) Decode(body []byte) (Request, error) {
	var r Request
	if len(body) > MaxBodyBytes {
		return r, &Error{"request_too_large", "Request body exceeds 4 MiB"}
	}
	if !boundedJSON(body) {
		return r, invalid()
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil || root == nil {
		return r, invalid()
	}
	var rawMessages []json.RawMessage
	if json.Unmarshal(root["messages"], &rawMessages) != nil || len(rawMessages) == 0 || len(rawMessages) > MaxMessages {
		return r, invalid()
	}
	r.Messages = make([]Message, 0, len(rawMessages))
	totalBlocks := 0
	for _, raw := range rawMessages {
		m, err := decodeMessage(raw)
		if err != nil {
			return Request{}, err
		}
		totalBlocks += len(m.Content)
		if totalBlocks > MaxBlocks {
			return Request{}, invalid()
		}
		r.Messages = append(r.Messages, m)
	}
	if err := validateTranscript(r.Messages); err != nil {
		return Request{}, err
	}
	tokens, err := estimateTokens(r.Messages)
	if err != nil {
		return Request{}, err
	}
	r.EstimatedTokens = tokens
	for mi := range r.Messages {
		for bi := range r.Messages[mi].Content {
			b := &r.Messages[mi].Content[bi]
			if b.Remote {
				b.Content = RemoteResultFallback
				b.Remote = false
				b.ContentLength = nil
			}
		}
	}
	if raw, ok := root["session_id"]; ok && string(raw) != "null" {
		if !strictString(raw, &r.SessionID) {
			return Request{}, invalid()
		}
	}
	if r.SessionID != "" {
		if !validUUID(r.SessionID) {
			return Request{}, invalid()
		}
		r.SessionID = strings.ToLower(r.SessionID)
	} else {
		r.SessionID, err = d.newSessionID()
		if err != nil {
			return Request{}, &Error{"internal_error", "Unable to create session"}
		}
	}
	// Invocation IDs are advisory metadata, not UUIDs.
	var invocation string
	if strictString(root["invocation_id"], &invocation) {
		r.InvocationID = &invocation
	}
	if d.Catalog.Default == "" || d.Catalog.Models[d.Catalog.Default] == "" {
		return Request{}, &Error{"internal_error", "Invalid model catalog"}
	}
	r.Context, r.Config = decodeAdvisory(root, d.Catalog)
	return r, nil
}

func strictString(raw json.RawMessage, out *string) bool {
	return len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, out) == nil
}

func decodeMessage(raw json.RawMessage) (Message, error) {
	var fields map[string]json.RawMessage
	var m Message
	if json.Unmarshal(raw, &fields) != nil || fields == nil || !strictString(fields["role"], &m.Role) || (m.Role != "user" && m.Role != "assistant") {
		return m, invalid()
	}
	content, hasContent := fields["content"]
	if hasContent {
		var text string
		if strictString(content, &text) {
			m.Content = []Block{{Type: "text", Text: text}}
		} else {
			blocks, err := decodeBlocks(content, false)
			if err != nil {
				return m, err
			}
			m.Content = blocks
		}
	}
	var legacy []Block
	if rawCalls, ok := fields["tool_calls"]; ok {
		var err error
		legacy, err = decodeBlocks(rawCalls, true)
		if err != nil || m.Role != "assistant" {
			return m, invalid()
		}
	}
	if !hasContent && len(legacy) == 0 {
		return m, invalid()
	}
	if len(legacy) > 0 {
		if len(m.Content) == 1 && m.Content[0].Type == "text" && m.Content[0].Text == "" {
			m.Content = nil
		}
		m.Content = append(m.Content, legacy...)
	}
	return m, nil
}

func decodeBlocks(raw json.RawMessage, legacy bool) ([]Block, error) {
	var items []json.RawMessage
	if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &items) != nil || len(items) > MaxBlocks {
		return nil, invalid()
	}
	blocks := make([]Block, 0, len(items))
	for _, item := range items {
		var f map[string]json.RawMessage
		if json.Unmarshal(item, &f) != nil || f == nil {
			return nil, invalid()
		}
		b := Block{Type: "tool_use"}
		if !legacy && !strictString(f["type"], &b.Type) {
			return nil, invalid()
		}
		switch b.Type {
		case "text":
			if !strictString(f["text"], &b.Text) {
				return nil, invalid()
			}
		case "tool_use":
			if !strictString(f["id"], &b.ID) || b.ID == "" || !strictString(f["name"], &b.Name) || b.Name == "" {
				return nil, invalid()
			}
			input, ok := f["input"]
			if !ok {
				return nil, invalid()
			}
			b.Input = input
		case "tool_result":
			if !strictString(f["tool_use_id"], &b.ToolUseID) || b.ToolUseID == "" {
				return nil, invalid()
			}
			if raw, ok := f["is_error"]; ok && (bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &b.IsError) != nil) {
				return nil, invalid()
			}
			if raw, ok := f["remote"]; ok && (bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &b.Remote) != nil) {
				return nil, invalid()
			}
			if b.Remote {
				if raw, ok := f["content_length"]; ok && string(raw) != "null" {
					var length int64
					if json.Unmarshal(raw, &length) != nil || length < 0 {
						return nil, invalid()
					}
					b.ContentLength = &length
				}
			} else if !strictString(f["content"], &b.Content) {
				return nil, invalid()
			}
		default:
			return nil, invalid()
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// boundedJSON caps parser depth and aggregate structural work before allocating
// RawMessage maps/slices. String contents do not consume the structural budget.
func boundedJSON(body []byte) bool {
	depth, work := 0, 0
	quoted, escaped := false, false
	for _, c := range body {
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			work++
		case '}', ']':
			depth--
			work++
		case ',', ':':
			work++
		}
		if depth < 0 || depth > MaxJSONDepth || work > MaxJSONStructure {
			return false
		}
	}
	return !quoted && depth == 0
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if s[i] != '-' {
				return false
			}
			continue
		}
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func (d Decoder) newSessionID() (string, error) {
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	random := d.Random
	if random == nil {
		random = rand.Reader
	}
	var b [16]byte
	if _, err := io.ReadFull(random, b[:]); err != nil {
		return "", err
	}
	ms := now().UnixMilli()
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	b[6] = b[6]&0x0f | 0x70
	b[8] = b[8]&0x3f | 0x80
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:]), nil
}
