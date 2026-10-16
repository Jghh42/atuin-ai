package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/jsoncheck"
	"github.com/Jghh42/atuin-ai/internal/safety"
)

// ErrInvalidBatch is deliberately free of provider output or arguments. The
// turn coordinator must convert it into a generic terminal stream error.
var ErrInvalidBatch = errors.New("invalid tool-call batch")

// ValidateBatch is a pure, atomic pre-emission gate. Call it ONLY after explicit
// successful provider completion; it cannot determine stream completion itself.
// The returned batch is independent of the input, including argument bytes.
func ValidateBatch(calls []Call, caps atuin.Capabilities) (Batch, error) {
	if len(calls) > config.MaxCallsPerResponse {
		return Batch{}, ErrInvalidBatch
	}
	if len(calls) == 0 {
		return Batch{Kind: TextOnly}, nil
	}
	allowed := make(map[string]bool)
	for _, def := range Definitions(caps) {
		allowed[def.Name] = true
	}
	seen := make(map[string]bool)
	out := Batch{Kind: LocalTools, Calls: make([]Call, 0, len(calls))}
	for _, call := range calls {
		if strings.TrimSpace(call.ID) == "" || seen[call.ID] || !allowed[call.Name] || len(call.Input) > config.MaxCallArgumentsBytes {
			return Batch{}, ErrInvalidBatch
		}
		seen[call.ID] = true
		fields, err := inputObject(call.Input)
		if err != nil || !validInput(call.Name, fields) {
			return Batch{}, ErrInvalidBatch
		}
		copied := Call{ID: call.ID, Name: call.Name, Input: bytes.Clone(call.Input)}
		if call.Name == "suggest_command" {
			// A suggestion is terminal. Never let accompanying calls reach the CLI,
			// which begins executing each tool_call immediately on receipt.
			if len(calls) != 1 {
				return Batch{}, ErrInvalidBatch
			}
			out.Kind = Suggestion
			danger, _ := fields["danger"].(string)
			notes, _ := fields["danger_notes"].(string)
			command, _ := fields["command"].(string)
			raised, warning := safety.Escalate(command, danger, notes)
			if raised != danger || warning != notes {
				var raw map[string]json.RawMessage
				if json.Unmarshal(call.Input, &raw) != nil {
					return Batch{}, ErrInvalidBatch
				}
				raw["danger"], _ = json.Marshal(raised)
				raw["danger_notes"], _ = json.Marshal(warning)
				copied.Input, err = json.Marshal(raw)
				if err != nil || len(copied.Input) > config.MaxCallArgumentsBytes {
					return Batch{}, ErrInvalidBatch
				}
			}
		}
		out.Calls = append(out.Calls, copied)
	}
	return out, nil
}

// Explicit validation mirrors the eight static tool schemas, not a general
// schema engine or extension mechanism. Unknown fields are retained, matching
// the advertised schemas' allowance of additional properties.
func validInput(name string, f map[string]any) bool {
	switch name {
	case "suggest_command":
		return requiredStrings(f, "command", "description") && enumString(f, "confidence", "high", "med", "low") && enumString(f, "danger", "high", "med", "low") && optionalString(f, "confidence_notes") && optionalString(f, "danger_notes")
	case "read_file":
		return requiredStrings(f, "file_path") && optionalInteger(f, "limit", 1, 1000) && optionalOffset(f)
	case "edit_file":
		return requiredStrings(f, "file_path", "old_string", "new_string") && optionalBoolean(f, "replace_all")
	case "write_file":
		return requiredStrings(f, "file_path", "content") && optionalBoolean(f, "overwrite")
	case "atuin_history":
		if !requiredStrings(f, "query") || !optionalInteger(f, "limit", 1, 50) {
			return false
		}
		modes, ok := f["filter_modes"].([]any)
		if !ok {
			return false
		}
		for _, mode := range modes {
			value, ok := mode.(string)
			if !ok || !oneOf(value, "workspace", "directory", "session", "host", "global") {
				return false
			}
		}
		return true
	case "atuin_output":
		if !requiredStrings(f, "history_id") {
			return false
		}
		if value, ok := f["ranges"]; ok {
			ranges, ok := value.([]any)
			if !ok || len(ranges) > 10 {
				return false
			}
			for _, value := range ranges {
				pair, ok := value.([]any)
				if !ok || len(pair) != 2 || !isInteger(pair[0]) || !isInteger(pair[1]) {
					return false
				}
			}
		}
		return true
	case "execute_shell_command":
		return requiredStrings(f, "command", "description") && enumString(f, "shell", "bash", "fish", "zsh", "nu") && optionalInteger(f, "timeout", 1, 600)
	case "load_skill":
		return requiredStrings(f, "name")
	}
	return false
}

func requiredStrings(f map[string]any, names ...string) bool {
	for _, name := range names {
		if _, ok := f[name].(string); !ok {
			return false
		}
	}
	return true
}
func optionalString(f map[string]any, name string) bool {
	if _, ok := f[name]; !ok {
		return true
	}
	return requiredStrings(f, name)
}
func optionalBoolean(f map[string]any, name string) bool {
	if value, ok := f[name]; ok {
		_, ok = value.(bool)
		return ok
	}
	return true
}
func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
func enumString(f map[string]any, name string, values ...string) bool {
	value, ok := f[name].(string)
	return ok && oneOf(value, values...)
}
func optionalInteger(f map[string]any, name string, min, max int64) bool {
	if value, ok := f[name]; ok {
		n, valid := integer(value)
		return valid && n >= min && n <= max
	}
	return true
}
func optionalOffset(f map[string]any) bool {
	value, exists := f["offset"]
	if !exists {
		return true
	}
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	// serde_json supports signed negative integers and unsigned positive
	// integers. The CLI defaults negative offsets to zero, as advertised.
	if strings.HasPrefix(string(number), "-") {
		return isInteger(value)
	}
	_, err := strconv.ParseUint(string(number), 10, 64)
	return err == nil
}
func isInteger(value any) bool { _, ok := integer(value); return ok }
func integer(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	// Range endpoints and history limits are signed 64-bit in the pinned CLI;
	// other bounded fields fit in that envelope. Fraction/exponent spellings
	// would be floats to serde_json and can be defaulted or rejected by the CLI.
	n, err := number.Int64()
	return n, err == nil
}

// Token walking rejects duplicate keys at every depth rather than letting
// server/client JSON decoders disagree on which value should be executed.
func inputObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || len(raw) > config.MaxCallArgumentsBytes || !jsoncheck.ValidUnicode(raw) {
		return nil, ErrInvalidBatch
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	budget := atuin.MaxJSONStructure
	value, err := inputValue(decoder, 0, &budget)
	if err != nil {
		return nil, ErrInvalidBatch
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalidBatch
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, ErrInvalidBatch
	}
	return object, nil
}

func inputValue(d *json.Decoder, depth int, budget *int) (any, error) {
	*budget -= 1
	if depth > atuin.MaxJSONDepth || *budget < 0 {
		return nil, ErrInvalidBatch
	}
	token, err := d.Token()
	if err != nil {
		return nil, ErrInvalidBatch
	}
	switch token {
	case json.Delim('{'):
		object := make(map[string]any)
		for d.More() {
			*budget -= 1
			key, err := d.Token()
			if err != nil || *budget < 0 {
				return nil, ErrInvalidBatch
			}
			name, ok := key.(string)
			if !ok {
				return nil, ErrInvalidBatch
			}
			if _, exists := object[name]; exists {
				return nil, ErrInvalidBatch
			}
			value, err := inputValue(d, depth+1, budget)
			if err != nil {
				return nil, ErrInvalidBatch
			}
			object[name] = value
		}
		closing, err := d.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, ErrInvalidBatch
		}
		return object, nil
	case json.Delim('['):
		array := make([]any, 0)
		for d.More() {
			value, err := inputValue(d, depth+1, budget)
			if err != nil {
				return nil, ErrInvalidBatch
			}
			array = append(array, value)
		}
		closing, err := d.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, ErrInvalidBatch
		}
		return array, nil
	case json.Delim('}'), json.Delim(']'):
		return nil, ErrInvalidBatch
	default:
		if number, ok := token.(json.Number); ok {
			// serde_json::Value rejects non-finite/overflowing numbers even
			// in unknown properties. Check compatibility without replacing
			// the original Number or rounding integer argument bytes.
			if _, err := strconv.ParseFloat(string(number), 64); err != nil {
				return nil, ErrInvalidBatch
			}
		}
		return token, nil
	}
}
