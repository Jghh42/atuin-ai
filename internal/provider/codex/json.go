package codex

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/jsoncheck"
)

// Validate before struct decoding: encoding/json otherwise accepts duplicate
// members and repairs lone surrogates. Token walking keeps numeric lexemes and
// bounds depth/work, including unknown additive metadata. Envelope keys also
// reject case-fold aliases because Go matches struct fields case-insensitively.
// Arguments are independent JSON objects with case-sensitive keys, as at the
// later tools.ValidateBatch gate.
func validStreamJSON(raw []byte, foldKeys bool) bool {
	if len(raw) == 0 || len(raw) > config.MaxProviderEventBytes || !jsoncheck.ValidUnicode(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	budget := 65536
	token := func() (json.Token, error) {
		budget--
		if budget < 0 {
			return nil, ErrStream
		}
		return d.Token()
	}
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 64 {
			return false
		}
		t, err := token()
		if err != nil {
			return false
		}
		switch t {
		case json.Delim('{'):
			keys := map[string]bool{}
			for d.More() {
				k, err := token()
				if err != nil {
					return false
				}
				key, ok := k.(string)
				if !ok {
					return false
				}
				if foldKeys {
					key = foldJSONKey(key)
				}
				if keys[key] {
					return false
				}
				keys[key] = true
				if !value(depth + 1) {
					return false
				}
			}
			end, err := token()
			return err == nil && end == json.Delim('}')
		case json.Delim('['):
			for d.More() {
				if !value(depth + 1) {
					return false
				}
			}
			end, err := token()
			return err == nil && end == json.Delim(']')
		case json.Delim('}'), json.Delim(']'):
			return false
		default:
			return true
		}
	}
	if !value(0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

// Match Go's Unicode simple-fold equivalence without quadratic key comparisons.
func foldJSONKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		min := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < min {
				min = next
			}
		}
		b.WriteRune(min)
	}
	return b.String()
}
