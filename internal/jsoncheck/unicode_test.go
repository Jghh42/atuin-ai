package jsoncheck

import "testing"

func TestValidUnicode(t *testing.T) {
	for _, raw := range []string{`{"a":"plain é🙂"}`, `{"\ud83d\ude42":"\ud83d\ude42"}`, `{"a":"\\ud800"}`, `{"a":"\"\\udfff"}`, `{"a":"\u0000"}`} {
		if !ValidUnicode([]byte(raw)) {
			t.Fatalf("valid Unicode rejected: %s", raw)
		}
	}
	for _, raw := range []string{`{"a":"\ud800"}`, `{"\udfff":"a"}`, `{"a":"\ud800\ud800"}`, `{"a":"\ud800\u0041"}`, `{"a":"\ud800x"}`, "{\"a\":\"\xff\"}"} {
		if ValidUnicode([]byte(raw)) {
			t.Fatalf("invalid Unicode accepted: %s", raw)
		}
	}
}
