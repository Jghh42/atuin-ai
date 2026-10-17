package setup

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

const MaxAtuinBytes = 64 << 10

var ErrTOML = errors.New("unsupported Atuin TOML layout or protocol; edit the three [ai] keys manually, retaining model")

func randomName() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrFile
	}
	return hex.EncodeToString(b[:]), nil
}

// ResolveAtuin never uses os.UserConfigDir: Atuin 18.19 on macOS uses the XDG
// ~/.config layout too. Newer source also recognizes ~/.atuin and explicit env
// roots. Multiple existing candidates are ambiguous, never silently shadowed.
func ResolveAtuin(explicit, home, xdg, configDir, atuinHome string, files Files) (string, error) {
	if explicit != "" {
		if !validPath(explicit) {
			return "", ErrFile
		}
		return explicit, nil
	}
	if !validPath(home) {
		return "", ErrFile
	}
	legacy := filepath.Join(home, ".config", "atuin", "config.toml")
	if xdg != "" {
		if !validPath(xdg) {
			return "", ErrFile
		}
		legacy = filepath.Join(xdg, "atuin", "config.toml")
	}
	paths := []string{legacy, filepath.Join(home, ".config", "atuin", "config.toml"), filepath.Join(home, ".atuin", "config.toml")}
	for _, root := range []string{configDir, atuinHome} {
		if root != "" {
			if !validPath(root) {
				return "", ErrFile
			}
			paths = append(paths, filepath.Join(root, "config.toml"))
		}
	}
	found := []string{}
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true
		s, e := files.Read(p, MaxAtuinBytes)
		if e != nil {
			return "", e
		}
		if s.Exists {
			found = append(found, p)
		}
	}
	if len(found) > 1 {
		return "", errors.New("multiple Atuin configurations; select --atuin-config PATH")
	}
	// Existing-file uniqueness does NOT identify the active file. Without an
	// installed-version proof, both known 18.19 and pinned newer rules must agree,
	// and that agreed path must be the sole existing file. Explicit selection is
	// required for creation or disagreement; never edit a lone ignored candidate.
	modern := filepath.Join(home, ".atuin", "config.toml")
	switch {
	case configDir != "":
		modern = filepath.Join(configDir, "config.toml")
	case atuinHome != "":
		modern = filepath.Join(atuinHome, "config.toml")
	case seenExisting(found, modern):
	case seenExisting(found, legacy):
		modern = legacy
	case seenExisting(found, filepath.Join(home, ".config", "atuin", "config.toml")):
		modern = legacy
	}
	if len(found) == 1 && found[0] == legacy && modern == legacy {
		return legacy, nil
	}
	return "", errors.New("active Atuin config is unproven; select --atuin-config PATH for the installed version")
}

func seenExisting(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

type span struct{ start, end int }

// statements recognizes strings (including multiline), comments and aggregate
// nesting; it refuses malformed balance even before TOML's semantic decode.
func statements(b []byte) ([]span, error) {
	var out []span
	start, depth := 0, 0
	var quote byte
	triple, escape, comment := false, false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if comment {
			if c != '\n' {
				continue
			}
			comment = false
		} else if quote != 0 {
			if quote == '"' && escape {
				escape = false
				continue
			}
			if quote == '"' && c == '\\' {
				escape = true
				continue
			}
			if c == quote {
				if triple {
					if i+2 < len(b) && b[i+1] == quote && b[i+2] == quote {
						i += 2
						quote = 0
						triple = false
					}
				} else {
					quote = 0
				}
			}
			continue
		} else {
			switch c {
			case '#':
				comment = true
			case '"', '\'':
				quote = c
				if i+2 < len(b) && b[i+1] == c && b[i+2] == c {
					triple = true
					i += 2
				}
			case '[', '{':
				depth++
			case ']', '}':
				depth--
				if depth < 0 {
					return nil, ErrTOML
				}
			}
		}
		if c == '\n' && depth == 0 && quote == 0 {
			out = append(out, span{start, i + 1})
			start = i + 1
		}
	}
	if quote != 0 || depth != 0 {
		return nil, ErrTOML
	}
	if start < len(b) {
		out = append(out, span{start, len(b)})
	}
	return out, nil
}

var simpleAssignment = regexp.MustCompile(`^([A-Za-z0-9_-]+)[ \t]*=`)
var simpleHeader = regexp.MustCompile(`^\[([A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*)\][ \t]*(?:#.*)?$`)

// valueEnd includes a complete multiline/aggregate value, but never a trailing
// comment. statements has already validated strings and nesting.
func valueEnd(s []byte) int {
	quote := byte(0)
	triple, escape := false, false
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if quote == '"' && escape {
				escape = false
				continue
			}
			if quote == '"' && c == '\\' {
				escape = true
				continue
			}
			if c == quote {
				if triple {
					if i+2 < len(s) && s[i+1] == quote && s[i+2] == quote {
						i += 2
						quote = 0
						triple = false
					}
				} else {
					quote = 0
				}
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			if i+2 < len(s) && s[i+1] == c && s[i+2] == c {
				triple = true
				i += 2
			}
			continue
		}
		if c == '[' || c == '{' {
			depth++
		}
		if c == ']' || c == '}' {
			depth--
		}
		if c == '#' && depth == 0 {
			return len(bytes.TrimRight(s[:i], " \t\r\n"))
		}
	}
	return len(bytes.TrimRight(s, " \t\r\n"))
}

type replacement struct {
	start, end int
	text       string
}

func PatchAtuin(data []byte, port int) ([]byte, error) {
	if len(data) > MaxAtuinBytes || !utf8.Valid(data) || port < 1 || port > 65535 {
		return nil, ErrTOML
	}
	var before map[string]any
	if _, e := toml.Decode(string(data), &before); e != nil {
		return nil, ErrTOML
	}
	if ai, ok := before["ai"]; ok {
		m, ok := ai.(map[string]any)
		if !ok {
			return nil, ErrTOML
		}
		if protocol, ok := m["endpoint_protocol"]; ok && protocol != "oss" {
			return nil, ErrTOML
		}
		// Aggregates may contain comments inside their span. They are also invalid
		// types for these settings; refuse rather than discarding embedded comments.
		for _, key := range []string{"enabled", "endpoint", "api_token"} {
			if value, exists := m[key]; exists {
				if key == "enabled" {
					if _, ok := value.(bool); !ok {
						return nil, ErrTOML
					}
				} else if _, ok := value.(string); !ok {
					return nil, ErrTOML
				}
			}
		}
	}
	stmts, e := statements(data)
	if e != nil {
		return nil, e
	}
	values := map[string]string{"enabled": "true", "endpoint": fmt.Sprintf("\"http://127.0.0.1:%d\"", port), "api_token": "\"\""}
	seen := map[string]bool{}
	inAI, hasAI := false, false
	insertAt := len(data)
	var edits []replacement
	for _, s := range stmts {
		raw := data[s.start:s.end]
		trimmed := strings.TrimSpace(string(raw))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			header := simpleHeader.FindStringSubmatch(trimmed)
			if inAI {
				insertAt = s.start
				inAI = false
			}
			if len(header) == 0 {
				var headerValue map[string]any
				if _, err := toml.Decode(trimmed+"\n__init_header_probe=0\n", &headerValue); err != nil {
					return nil, ErrTOML
				}
				if _, ok := headerValue["ai"]; ok {
					return nil, ErrTOML
				}
				continue
			}
			if strings.HasPrefix(header[1], "ai.") {
				return nil, ErrTOML
			}
			if header[1] == "ai" {
				if hasAI {
					return nil, ErrTOML
				}
				hasAI = true
				inAI = true
				insertAt = len(data)
			}
			continue
		}
		if !inAI {
			// Refuse root dotted/inline AI, even when followed by an explicit [ai].
			var statement map[string]any
			if _, err := toml.Decode(string(raw), &statement); err == nil {
				if _, ok := statement["ai"]; ok {
					return nil, ErrTOML
				}
			}
			continue
		}
		// Leading indentation is legal and preserved.
		lead := len(raw) - len(bytes.TrimLeft(raw, " \t"))
		match := simpleAssignment.FindSubmatchIndex(raw[lead:])
		if match == nil {
			return nil, ErrTOML
		}
		key := string(raw[lead+match[2] : lead+match[3]])
		value, authorized := values[key]
		if !authorized {
			continue
		}
		if seen[key] {
			return nil, ErrTOML
		}
		seen[key] = true
		start := lead + match[1]
		start += len(raw[start:]) - len(bytes.TrimLeft(raw[start:], " \t"))
		end := start + valueEnd(raw[start:])
		edits = append(edits, replacement{s.start + start, s.start + end, value})
	}
	if _, ok := before["ai"]; ok && !hasAI {
		return nil, ErrTOML
	}
	addition := ""
	for _, key := range []string{"enabled", "endpoint", "api_token"} {
		if !seen[key] {
			addition += key + " = " + values[key] + "\n"
		}
	}
	if !hasAI {
		if len(data) > 0 && data[len(data)-1] != '\n' {
			addition = "\n" + addition
		}
		addition = "\n[ai]\n" + addition
		insertAt = len(data)
	} else if addition != "" && insertAt > 0 && data[insertAt-1] != '\n' {
		addition = "\n" + addition
	}
	if addition != "" {
		edits = append(edits, replacement{insertAt, insertAt, addition})
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	result := append([]byte(nil), data...)
	for _, edit := range edits {
		result = append(append(append([]byte(nil), result[:edit.start]...), []byte(edit.text)...), result[edit.end:]...)
	}
	if len(result) > MaxAtuinBytes {
		return nil, ErrTOML
	}
	var after map[string]any
	if _, e := toml.Decode(string(result), &after); e != nil {
		return nil, ErrTOML
	}
	// A surgical lexer bug must never authorize an unrelated semantic change.
	scrub := func(m map[string]any) {
		if ai, ok := m["ai"].(map[string]any); ok {
			for k := range values {
				delete(ai, k)
			}
			if len(ai) == 0 {
				delete(m, "ai")
			}
		}
	}
	scrub(before)
	scrub(after)
	if !semanticEqual(reflect.ValueOf(before), reflect.ValueOf(after)) {
		return nil, ErrTOML
	}
	return result, nil
}

// TOML permits NaN. Equal unchanged NaNs must not defeat the unrelated-value
// guard, while every map key/type/array element still must match exactly.
func semanticEqual(a, b reflect.Value) bool {
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return semanticEqual(a.Elem(), b.Elem())
	case reflect.Map:
		if a.Len() != b.Len() {
			return false
		}
		for _, k := range a.MapKeys() {
			if !semanticEqual(a.MapIndex(k), b.MapIndex(k)) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !semanticEqual(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Float64, reflect.Float32:
		return a.Float() == b.Float() || (math.IsNaN(a.Float()) && math.IsNaN(b.Float()))
	default:
		return reflect.DeepEqual(a.Interface(), b.Interface())
	}
}
func ConfigureAtuin(files Files, path string, port int) error {
	old, e := files.Read(path, MaxAtuinBytes)
	if e != nil {
		return e
	}
	data, e := PatchAtuin(old.Data, port)
	if e != nil {
		return e
	}
	if old.Exists && bytes.Equal(old.Data, data) {
		return nil
	}
	write, err := files.Replace(path, old, data, 0600, true)
	if err == nil && write.Applied && write.Durable && write.After.Exists && bytes.Equal(write.After.Data, data) {
		return nil
	}
	if err == nil {
		err = ErrCommitUncertain
	}
	return errors.Join(err, rollbackWrite(files, path, old, write))
}
