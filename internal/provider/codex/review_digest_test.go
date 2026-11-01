package codex

import (
	"crypto/sha256"
	"encoding/json"
	"testing"
)

func TestReviewIncrementalInitialDigestPreservesLegacyEquality(t *testing.T) {
	samples := []outputItem{{Type: "message", ID: "m"}, {Type: "message", ID: "m", Namespace: json.RawMessage(`null`)}, {Type: "message", ID: "m", Content: outputContent{}}, {Type: "message", ID: "m", Name: "x"}, {Type: "message", ID: "m", CallID: "x"}, {Type: "message", ID: "m", Arguments: "x"}, {Type: "message", ID: "m", Content: outputContent{{Type: "output_text", Text: ""}}}, {Type: "message", ID: "m", Content: outputContent{{Type: "output_text", Text: "é<&\n"}}}}
	for _, a := range samples {
		for _, b := range samples {
			left, _ := json.Marshal(a)
			right, _ := json.Marshal(b)
			if (sha256.Sum256(left) == sha256.Sum256(right)) != (itemDigest(a, false) == itemDigest(b, false)) {
				t.Fatal("legacy decoded snapshot equality changed")
			}
		}
	}
}
func TestReviewMetadataStreamingDigestCanonicalAndUnambiguous(t *testing.T) {
	decode := func(raw string) [32]byte {
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &fields) != nil {
			t.Fatal("fixture")
		}
		return metadataDigest(fields, "text", "content")
	}
	if decode(`{"x":{"a":"é","b":[0,1]}}`) != decode(`{"x":{"b":[0,1],"a":"\u00e9"}}`) {
		t.Fatal("metadata object order/decoded strings changed")
	}
	for _, pair := range [][2]string{{`{"x":[{},null]}`, `{"x":[{"end-object":null}]}`}, {`{"x":1}`, `{"x":"1"}`}, {`{"x":[]}`, `{"x":{}}`}, {`{"x":[0,1]}`, `{"x":[1,0]}`}, {`{}`, `{"text":""}`}} {
		if decode(pair[0]) == decode(pair[1]) {
			t.Fatal("metadata grammar or payload shape alias")
		}
	}
}
