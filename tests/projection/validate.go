// Package projection is an independent test oracle for outbound request meaning.
// It imports no production decoder/projector/tool registry. Contracts derive from
// pinned core domain/tools.gleam and openai_compat.gleam; prose is not compared.
package projection

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
)

const Model = "gpt-6.1-sol"

var order = []string{"suggest_command", "read_file", "edit_file", "write_file", "atuin_history", "atuin_output", "execute_shell_command", "load_skill"}

const schemaJSON = `{
"suggest_command":{"type":"object","properties":{"command":{"type":"string"},"description":{"type":"string"},"confidence":{"type":"string","enum":["high","med","low"]},"confidence_notes":{"type":"string"},"danger":{"type":"string","enum":["high","med","low"]},"danger_notes":{"type":"string"}},"required":["command","description","confidence","danger"]},
"read_file":{"type":"object","properties":{"file_path":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":1000},"offset":{"type":"integer"}},"required":["file_path"]},
"edit_file":{"type":"object","properties":{"file_path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["file_path","old_string","new_string"]},
"write_file":{"type":"object","properties":{"file_path":{"type":"string"},"content":{"type":"string"},"overwrite":{"type":"boolean"}},"required":["file_path","content"]},
"atuin_history":{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50},"filter_modes":{"type":"array","items":{"type":"string","enum":["workspace","directory","session","host","global"]}}},"required":["query","filter_modes"]},
"atuin_output":{"type":"object","properties":{"history_id":{"type":"string"},"ranges":{"type":"array","maxItems":10,"items":{"type":"array","minItems":2,"maxItems":2,"items":{"type":"integer"}}}},"required":["history_id"]},
"execute_shell_command":{"type":"object","properties":{"command":{"type":"string"},"shell":{"type":"string","enum":["bash","fish","zsh","nu"]},"timeout":{"type":"integer","minimum":1,"maximum":600},"description":{"type":"string"}},"required":["command","shell","description"]},
"load_skill":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}}
`

func decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e := d.Decode(v); e != nil {
		return e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func cleanSchema(raw any) any {
	m, ok := raw.(map[string]any)
	if !ok {
		return raw
	}
	out := map[string]any{}
	for k, v := range m {
		if k == "description" {
			continue
		}
		switch k {
		case "properties":
			props, ok := v.(map[string]any)
			if !ok {
				out[k] = v
				continue
			}
			p := map[string]any{}
			for name, schema := range props {
				p[name] = cleanSchema(schema)
			}
			out[k] = p
		case "items":
			out[k] = cleanSchema(v)
		default:
			out[k] = v
		}
	}
	return out
}

type block struct {
	Type, Text, ID, Name, ToolUseID, Content string
	Input                                    any
	Error                                    bool
}
type message struct {
	Role   string
	Blocks []block
	Plain  bool
}

func normalized(raw map[string]any) ([]message, error) {
	list, ok := raw["messages"].([]any)
	if !ok {
		return nil, fmt.Errorf("messages missing")
	}
	out := []message{}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("message type")
		}
		role, _ := m["role"].(string)
		if role != "user" && role != "assistant" {
			return nil, fmt.Errorf("unsupported message role")
		}
		msg := message{Role: role}
		switch content := m["content"].(type) {
		case string:
			msg.Plain = true
			if content != "" {
				msg.Blocks = append(msg.Blocks, block{Type: "text", Text: content})
			}
		case []any:
			for _, part := range content {
				p, ok := part.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("block type")
				}
				str := func(k string) string { s, _ := p[k].(string); return s }
				b := block{Type: str("type"), Text: str("text"), ID: str("id"), Name: str("name"), ToolUseID: str("tool_use_id"), Input: p["input"], Content: str("content")}
				b.Error, _ = p["is_error"].(bool)
				if remote, _ := p["remote"].(bool); remote {
					b.Content = "[Tool result no longer available]"
				}
				msg.Blocks = append(msg.Blocks, b)
			}
		case nil:
		default:
			return nil, fmt.Errorf("content type")
		}
		if calls, ok := m["tool_calls"].([]any); ok {
			for _, c := range calls {
				v, ok := c.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("legacy call")
				}
				id, _ := v["id"].(string)
				name, _ := v["name"].(string)
				input := v["input"]
				if f, ok := v["function"].(map[string]any); ok {
					name, _ = f["name"].(string)
					arguments, _ := f["arguments"].(string)
					if decode([]byte(arguments), &input) != nil {
						return nil, fmt.Errorf("legacy arguments")
					}
				}
				msg.Blocks = append(msg.Blocks, block{Type: "tool_use", ID: id, Name: name, Input: input})
			}
		}
		out = append(out, msg)
	}
	seen, pending := map[string]bool{}, map[string]bool{}
	for _, m := range out {
		for _, b := range m.Blocks {
			switch b.Type {
			case "text":
			case "tool_use":
				if _, ok := b.Input.(map[string]any); !ok || m.Role != "assistant" || b.ID == "" || b.Name == "" || seen[b.ID] {
					return nil, fmt.Errorf("invalid historical call")
				}
				seen[b.ID] = true
				if b.Name != "suggest_command" {
					pending[b.ID] = true
				}
			case "tool_result":
				if m.Role != "user" || !pending[b.ToolUseID] {
					return nil, fmt.Errorf("invalid historical result correlation")
				}
				delete(pending, b.ToolUseID)
			default:
				return nil, fmt.Errorf("unsupported history block")
			}
		}
	}
	if len(pending) != 0 {
		return nil, fmt.Errorf("unresolved historical calls")
	}
	return out, nil
}
func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }
func nativeHistory(messages []message) []any {
	out := []any{}
	for _, m := range messages {
		for _, b := range m.Blocks {
			switch b.Type {
			case "text":
				out = append(out, map[string]any{"role": m.Role, "content": b.Text})
			case "tool_use":
				if b.Name == "suggest_command" {
					out = append(out, map[string]any{"role": "assistant", "content": "Terminal command suggestion (suggest_command; suggestion only, not evidence of execution). Original arguments JSON: " + jsonString(b.Input)})
				} else {
					out = append(out, map[string]any{"type": "function_call", "call_id": b.ID, "name": b.Name, "arguments": b.Input})
				}
			case "tool_result":
				text := b.Content
				if b.Error {
					text = "Tool execution failed: " + text
				}
				out = append(out, map[string]any{"type": "function_call_output", "call_id": b.ToolUseID, "output": text})
			}
		}
	}
	return out
}
func ccHistory(messages []message) []any {
	out := []any{}
	for index, m := range messages {
		parts, calls := []any{}, []any{}
		flush := func() {
			if len(parts) > 0 || len(calls) > 0 {
				var content any
				if len(parts) > 0 {
					content = parts
				}
				v := map[string]any{"role": m.Role, "content": content}
				if len(calls) > 0 {
					v["tool_calls"] = calls
				}
				out = append(out, v)
				parts, calls = []any{}, []any{}
			}
		}
		for _, b := range m.Blocks {
			switch b.Type {
			case "text":
				if b.Text != "" {
					parts = append(parts, map[string]any{"type": "text", "text": b.Text})
				}
			case "tool_use":
				calls = append(calls, map[string]any{"id": b.ID, "type": "function", "function": map[string]any{"name": b.Name, "arguments": b.Input}})
			case "tool_result":
				flush()
				out = append(out, map[string]any{"role": "tool", "tool_call_id": b.ToolUseID, "content": b.Content})
			}
		}
		flush()
		if index+1 < len(messages) && messages[index+1].Role == "user" && messages[index+1].Plain {
			for _, b := range m.Blocks {
				if b.Type == "tool_use" && b.Name == "suggest_command" {
					out = append(out, map[string]any{"role": "tool", "tool_call_id": b.ID, "content": `{"success": true}`})
				}
			}
		}
	}
	return out
}
func parseArguments(items []any) error {
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("projected item type")
		}
		const suggestionPrefix = "Terminal command suggestion (suggest_command; suggestion only, not evidence of execution). Original arguments JSON: "
		if content, ok := m["content"].(string); ok && m["role"] == "assistant" && strings.HasPrefix(content, suggestionPrefix) {
			var v any
			if decode([]byte(strings.TrimPrefix(content, suggestionPrefix)), &v) != nil {
				return fmt.Errorf("suggestion history JSON")
			}
			m["content"] = suggestionPrefix + jsonString(v)
		}
		if m["type"] == "function_call" {
			s, ok := m["arguments"].(string)
			if !ok {
				return fmt.Errorf("arguments not string")
			}
			var v any
			if decode([]byte(s), &v) != nil {
				return fmt.Errorf("bad arguments")
			}
			m["arguments"] = v
		}
		if list, ok := m["tool_calls"].([]any); ok {
			for _, call := range list {
				c, ok := call.(map[string]any)
				if !ok {
					return fmt.Errorf("call type")
				}
				f, ok := c["function"].(map[string]any)
				if !ok {
					return fmt.Errorf("function missing")
				}
				s, ok := f["arguments"].(string)
				if !ok {
					return fmt.Errorf("CC arguments not string")
				}
				var v any
				if decode([]byte(s), &v) != nil {
					return fmt.Errorf("CC arguments JSON")
				}
				f["arguments"] = v
			}
		}
	}
	return nil
}
func contextText(item any, cc bool) (string, error) {
	m, ok := item.(map[string]any)
	if !ok || m["role"] != "user" {
		return "", fmt.Errorf("context role/position")
	}
	if !cc {
		s, ok := m["content"].(string)
		if !ok {
			return "", fmt.Errorf("context content")
		}
		return s, nil
	}
	p, ok := m["content"].([]any)
	if !ok || len(p) != 1 {
		return "", fmt.Errorf("CC context parts")
	}
	v, ok := p[0].(map[string]any)
	if !ok || v["type"] != "text" {
		return "", fmt.Errorf("CC context text")
	}
	s, _ := v["text"].(string)
	return s, nil
}
func validateContext(text string, raw map[string]any, cc bool) error {
	start := strings.Index(text, "<turn_context>")
	if start < 0 {
		return fmt.Errorf("missing context envelope")
	}
	var value struct {
		Date string  `xml:"current_date_utc"`
		PWD  *string `xml:"current_directory"`
		Last *string `xml:"last_command"`
	}
	if xml.Unmarshal([]byte(text[start:]), &value) != nil {
		return fmt.Errorf("invalid context XML")
	}
	layout := "2006-01-02"
	if cc {
		layout = "January 02, 2006"
	}
	if _, e := time.Parse(layout, value.Date); e != nil {
		return fmt.Errorf("context date shape")
	}
	ctx, _ := raw["context"].(map[string]any)
	for _, field := range []struct {
		key string
		got *string
	}{{"pwd", value.PWD}, {"last_command", value.Last}} {
		expected, exists := ctx[field.key].(string)
		if exists {
			if field.got == nil || *field.got != expected {
				return fmt.Errorf("context %s changed", field.key)
			}
		} else if field.got != nil {
			return fmt.Errorf("unexpected context %s", field.key)
		}
	}
	return nil
}

// Validate checks model, exact ordered declarations and complete ordered history
// BEFORE either fake may return canned success. Prompt prose may differ; context
// envelope/data and dialect-specific projection remain contractual.
func Validate(dialect string, atuin, outbound []byte) error {
	var raw, wire map[string]any
	if decode(atuin, &raw) != nil || decode(outbound, &wire) != nil {
		return fmt.Errorf("invalid request JSON")
	}
	cc := dialect == "chat-completions"
	if !cc && dialect != "responses" {
		return fmt.Errorf("unknown dialect")
	}
	cfg, _ := raw["config"].(map[string]any)
	selected := Model
	if cfg["model"] == "gpt-6-luna" {
		selected = "gpt-6-luna"
	}
	if wire["model"] != selected || wire["stream"] != true {
		return fmt.Errorf("model/stream changed")
	}
	if !cc {
		if wire["store"] != false || wire["previous_response_id"] != nil {
			return fmt.Errorf("stateless request changed")
		}
		if s, ok := wire["instructions"].(string); !ok || strings.TrimSpace(s) == "" {
			return fmt.Errorf("instructions missing")
		}
	}
	caps := map[string]bool{}
	if list, ok := cfg["capabilities"].([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok {
				caps[strings.TrimSpace(s)] = true
			}
		}
	}
	expected := []string{}
	for i, name := range order {
		if i == 0 || caps["client_v1_"+name] {
			expected = append(expected, name)
		}
	}
	definitions, ok := wire["tools"].([]any)
	if !ok || len(definitions) != len(expected) {
		return fmt.Errorf("enabled tool count changed")
	}
	var schemas map[string]any
	_ = decode([]byte(schemaJSON), &schemas)
	for i, name := range expected {
		d, ok := definitions[i].(map[string]any)
		if !ok || d["type"] != "function" {
			return fmt.Errorf("tool wrapper %d", i)
		}
		if cc {
			if len(d) != 2 {
				return fmt.Errorf("CC wrapper extras")
			}
			d, ok = d["function"].(map[string]any)
			if !ok {
				return fmt.Errorf("CC function missing")
			}
		} else {
			if _, exists := d["strict"]; !exists || d["strict"] != nil {
				return fmt.Errorf("strict contract changed")
			}
		}
		if d["name"] != name {
			return fmt.Errorf("tool order/name %d", i)
		}
		if description, ok := d["description"].(string); !ok || description == "" {
			return fmt.Errorf("tool description missing")
		}
		wantLen := 3
		if !cc {
			wantLen = 5
		}
		if len(d) != wantLen {
			return fmt.Errorf("tool declaration extras %s", name)
		}
		if !reflect.DeepEqual(cleanSchema(d["parameters"]), schemas[name]) {
			return fmt.Errorf("tool schema changed: %s", name)
		}
	}
	messages, e := normalized(raw)
	if e != nil {
		return e
	}
	var actual, want []any
	if cc {
		list, ok := wire["messages"].([]any)
		if !ok || len(list) < 2 {
			return fmt.Errorf("CC prompt/context missing")
		}
		system, ok := list[0].(map[string]any)
		if !ok || system["role"] != "system" {
			return fmt.Errorf("system envelope position")
		}
		systemCopy := map[string]any{"role": "user", "content": system["content"]}
		systemText, systemErr := contextText(systemCopy, true)
		if systemErr != nil || strings.TrimSpace(systemText) == "" {
			return fmt.Errorf("system prose empty")
		}
		text, e := contextText(list[1], true)
		if e != nil {
			return e
		}
		if e = validateContext(text, raw, true); e != nil {
			return e
		}
		actual = list[2:]
		want = ccHistory(messages)
	} else {
		list, ok := wire["input"].([]any)
		if !ok || len(list) < 1 {
			return fmt.Errorf("responses context missing")
		}
		text, e := contextText(list[0], false)
		if e != nil {
			return e
		}
		if e = validateContext(text, raw, false); e != nil {
			return e
		}
		actual = list[1:]
		want = nativeHistory(messages)
	}
	if e = parseArguments(actual); e != nil {
		return e
	}
	if !reflect.DeepEqual(actual, want) {
		return fmt.Errorf("ordered full history changed")
	}
	return nil
}
