// Package jsoncheck contains shared lossless-JSON string checks.
package jsoncheck

import (
	"strconv"
	"unicode/utf8"
)

// ValidUnicode rejects invalid UTF-8 and lone UTF-16 surrogate escapes that
// Go would replace but the pinned Rust client rejects. It checks every string,
// including unknown nested values and object keys, without modifying raw JSON.
// This only validates Unicode; callers must also validate JSON syntax.
func ValidUnicode(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	quoted := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || raw[i] != '\\' {
			continue
		}
		if i+1 >= len(raw) {
			return false
		}
		if raw[i+1] != 'u' {
			i++
			continue
		}
		if i+6 > len(raw) {
			return false
		}
		code, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			return false
		}
		if code >= 0xd800 && code <= 0xdbff {
			if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+8:i+12]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 11
		} else {
			if code >= 0xdc00 && code <= 0xdfff {
				return false
			}
			i += 5
		}
	}
	return true
}
