package credentials

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"github.com/Jghh42/atuin-ai/internal/jsoncheck"
)

// foldedJSONName uses the least rune in each Unicode SimpleFold orbit.
// Equal canonical strings are exactly Unicode EqualFold aliases, matching
// encoding/json's case-insensitive struct-field selection (including ſ and K).
// Lowercasing alone does not cover those orbits.
func foldedJSONName(name string) string {
	return strings.Map(func(r rune) rune {
		smallest := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < smallest {
				smallest = f
			}
		}
		return smallest
	}, name)
}

// Reject ambiguous duplicate members, invalid Unicode and excessive nesting.
func decodeJSON(b []byte, out any, strict bool) error {
	if len(b) > maxRecordBytes || !jsoncheck.ValidUnicode(b) {
		return ErrOAuth
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 16 {
			return ErrOAuth
		}
		t, e := d.Token()
		if e != nil {
			return ErrOAuth
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return ErrOAuth
					}
					s, ok := k.(string)
					if !ok {
						return ErrOAuth
					}
					folded := foldedJSONName(s)
					if seen[folded] {
						return ErrOAuth
					}
					seen[folded] = true
					if walk(depth+1) != nil {
						return ErrOAuth
					}
				}
				t, e = d.Token()
				if e != nil || t != json.Delim('}') {
					return ErrOAuth
				}
			case '[':
				for d.More() {
					if walk(depth+1) != nil {
						return ErrOAuth
					}
				}
				t, e = d.Token()
				if e != nil || t != json.Delim(']') {
					return ErrOAuth
				}
			default:
				return ErrOAuth
			}
		}
		return nil
	}
	if walk(0) != nil {
		return ErrOAuth
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrOAuth
	}
	if strict {
		// encoding/json accepts case-insensitive struct aliases; the persisted
		// schema does not. Require exact names and explicit non-optional fields.
		var fields map[string]json.RawMessage
		if json.Unmarshal(b, &fields) != nil || fields == nil {
			return ErrOAuth
		}
		allowed := map[string]bool{}
		for _, name := range []string{"schema", "provider", "registration", "state", "generation", "auth_epoch", "account", "access", "refresh", "issued_at", "saved_at", "expires_at"} {
			allowed[name] = true
		}
		for name := range fields {
			if !allowed[name] {
				return ErrOAuth
			}
		}
		for _, name := range []string{"schema", "provider", "registration", "state", "generation", "auth_epoch", "issued_at", "saved_at", "expires_at"} {
			if _, ok := fields[name]; !ok {
				return ErrOAuth
			}
		}
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if strict {
		d.DisallowUnknownFields()
	}
	if d.Decode(out) != nil {
		return ErrOAuth
	}
	return nil
}
