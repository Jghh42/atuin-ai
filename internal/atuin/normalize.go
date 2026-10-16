package atuin

import (
	"bytes"
	"encoding/json"
	"github.com/rivo/uniseg"
)

// validateTranscript refuses ambiguous full-history replay instead of silently
// deleting calls or inventing outputs. Correlation is by ID, not message
// adjacency: the CLI may interleave results and further assistant call groups.
// suggest_command is a terminal answer and therefore needs no tool result.
func validateTranscript(messages []Message) error {
	seen := make(map[string]bool)
	pending := make(map[string]bool)
	for _, m := range messages {
		for _, b := range m.Content {
			switch b.Type {
			case "tool_use":
				if m.Role != "assistant" || seen[b.ID] {
					return invalid()
				}
				seen[b.ID] = true
				if b.Name != "suggest_command" {
					pending[b.ID] = true
				}
			case "tool_result":
				if m.Role != "user" || !pending[b.ToolUseID] {
					return invalid()
				}
				delete(pending, b.ToolUseID)
			}
		}
	}
	if len(pending) != 0 {
		return invalid()
	}
	return nil
}

// Estimate before remote-result hydration. Saturating arithmetic prevents an
// untrusted content_length from overflowing, even on a 32-bit host. Like core,
// division is floor(total characters / 4), not a per-block rounded estimate.
func estimateTokens(messages []Message) (int, error) {
	const maxChars int64 = (MaxConversationTokens+1)*4 - 1
	var total int64
	for _, m := range messages {
		for _, b := range m.Content {
			var n int64
			switch b.Type {
			case "text":
				n = int64(uniseg.GraphemeClusterCount(b.Text))
			case "tool_use":
				var compact bytes.Buffer
				// Decode with UseNumber to normalize escaped strings without losing
				// integer precision. Numeric spellings remain intentionally intact.
				decoder := json.NewDecoder(bytes.NewReader(b.Input))
				decoder.UseNumber()
				var value any
				if decoder.Decode(&value) != nil {
					return 0, invalid()
				}
				encoder := json.NewEncoder(&compact)
				encoder.SetEscapeHTML(false)
				if encoder.Encode(value) != nil {
					return 0, invalid()
				}
				n = int64(uniseg.GraphemeClusterCount(string(bytes.TrimSuffix(compact.Bytes(), []byte{'\n'}))))
			case "tool_result":
				if b.Remote {
					if b.ContentLength != nil {
						n = *b.ContentLength
					}
				} else {
					n = int64(uniseg.GraphemeClusterCount(b.Content))
				}
			}
			if n > maxChars-total {
				return 0, &Error{"conversation_too_large", "Conversation exceeds 180000 estimated tokens"}
			}
			total += n
		}
	}
	return int(total / 4), nil
}
