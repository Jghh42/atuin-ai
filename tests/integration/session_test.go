package integration

import (
	"reflect"
	"testing"

	"github.com/Jghh42/atuin-ai/tests/compatfixture"
)

func TestSessionValidationPrecedesNormalization(t *testing.T) {
	const a = "01900000-0000-7000-8000-000000000001"
	const b = "01900000-0000-7000-8000-000000000002"
	valid := func(id any) []compatfixture.Event {
		return []compatfixture.Event{{Event: "status", Data: map[string]any{"state": "processing"}}, {Event: "done", Data: map[string]any{"session_id": id, "usage": map[string]any{}}}}
	}
	if e := compatfixture.ValidateSession(valid(a), a, a); e != nil {
		t.Fatal(e)
	}
	for _, v := range []any{nil, "", "malformed", "01900000"} {
		t.Run("bad-event", func(t *testing.T) {
			events := valid(v)
			if compatfixture.ValidateSession(events, a, "") == nil {
				t.Fatal("invalid done session accepted")
			}
			normalized := compatfixture.Normalize(events)
			if !reflect.DeepEqual(normalized[1].Data["session_id"], v) {
				t.Fatal("invalid ID normalized into valid")
			}
		})
	}
	missing := valid(a)
	delete(missing[1].Data, "session_id")
	if compatfixture.ValidateSession(missing, a, "") == nil {
		t.Fatal("missing done session accepted")
	}
	for _, header := range []string{"", "bad", b} {
		if compatfixture.ValidateSession(valid(a), header, "") == nil {
			t.Fatal("missing/mismatched header accepted")
		}
	}
	if compatfixture.ValidateSession(valid(b), b, a) == nil {
		t.Fatal("explicit continuation identity changed")
	}
	changing := []compatfixture.Event{{Event: "status", Data: map[string]any{"session_id": a}}, {Event: "done", Data: map[string]any{"session_id": b}}}
	if compatfixture.ValidateSession(changing, a, "") == nil {
		t.Fatal("changing IDs erased by normalization")
	}
	for _, raw := range []string{`{"session_id":true}`, `{"session_id":12}`, `{"session_id":{}}`, `{"session_id":"bad"}`} {
		if compatfixture.ValidateSession(valid(a), a, compatfixture.RequestedSession([]byte(raw))) == nil {
			t.Fatal("malformed explicit request session treated as unspecified")
		}
	}
	for _, raw := range []string{`{}`, `{"session_id":null}`, `{"session_id":""}`} {
		if e := compatfixture.ValidateSession(valid(a), a, compatfixture.RequestedSession([]byte(raw))); e != nil {
			t.Fatal("valid absent/new session rejected", e)
		}
	}
	duplicate := append(valid(a), valid(a)[1])
	if compatfixture.ValidateSession(duplicate, a, "") == nil {
		t.Fatal("duplicate terminal accepted")
	}
}
