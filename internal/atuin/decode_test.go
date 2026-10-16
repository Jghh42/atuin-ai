package atuin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func testDecoder() Decoder {
	return Decoder{Catalog: ModelCatalog{Default: "default", Models: map[string]string{"default": "provider-default", "fast": "provider-fast"}}, Now: func() time.Time { return time.UnixMilli(0x123456789abc) }, Random: bytes.NewReader(make([]byte, 16))}
}

func errorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestCompatibilityRequests(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compat/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string
		Request json.RawMessage
		Tokens  int
		Error   string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			r, err := testDecoder().Decode(tc.Request)
			if errorCode(err) != tc.Error {
				t.Fatalf("code=%q, want %q", errorCode(err), tc.Error)
			}
			if err != nil {
				return
			}
			if r.EstimatedTokens != tc.Tokens {
				t.Errorf("tokens=%d want %d", r.EstimatedTokens, tc.Tokens)
			}
			if r.SessionID != "12345678-9abc-7000-8000-000000000000" {
				t.Errorf("UUIDv7=%s", r.SessionID)
			}
			for _, m := range r.Messages {
				for _, b := range m.Content {
					if b.Remote {
						t.Error("remote result not hydrated")
					}
					if tc.Name == "remote_result" && b.Type == "tool_result" && (b.Content != RemoteResultFallback || !b.IsError) {
						t.Error("lost remote fallback/error")
					}
					if tc.Name == "text_tool_use_and_inline_result" && b.Type == "tool_use" && string(b.Input) != `{"n":9007199254740993}` {
						t.Errorf("numeric precision: %s", b.Input)
					}
				}
			}
		})
	}
}

func TestStrictMessageValidation(t *testing.T) {
	cases := []string{
		`null`, `[]`, `{}`, `{} {}`, `{"messages":null}`,
		`{"messages":[{"role":1,"content":"hi"}]}`,
		`{"messages":[{"role":"user","content":false}]}`,
		`{"messages":[{"role":"assistant","tool_calls":[]}]}`,
		`{"messages":[{"role":"user","content":"hi","tool_calls":[]}]}`,
		`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"","name":"suggest_command","input":null}]}]}`,
		`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"suggest_command"}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"tool_use","id":"c","name":"suggest_command","input":{}}]}]}`,
		`{"messages":[{"role":"assistant","content":[{"type":"text","text":null}]}]}`,
	}
	for i, s := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := testDecoder().Decode([]byte(s)); errorCode(err) != "invalid_request" {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func exchange(result string) string {
	return `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"read_file","input":{}}]},{"role":"user","content":[` + result + `]}]}`
}

func TestResultValidation(t *testing.T) {
	for _, result := range []string{
		`{"type":"tool_result","tool_use_id":"c","content":null}`,
		`{"type":"tool_result","tool_use_id":"c","content":"ok","is_error":null}`,
		`{"type":"tool_result","tool_use_id":"c","content":"ok","remote":"true"}`,
		`{"type":"tool_result","tool_use_id":"c","remote":true,"content_length":-1}`,
		`{"type":"tool_result","tool_use_id":"c","remote":true,"content_length":1.1}`,
		`{"type":"tool_result","tool_use_id":"c","remote":true,"content_length":9223372036854775808}`,
	} {
		if _, err := testDecoder().Decode([]byte(exchange(result))); errorCode(err) != "invalid_request" {
			t.Errorf("accepted %s: %v", result, err)
		}
	}
	for _, length := range []string{"null", "0", "40"} {
		r, err := testDecoder().Decode([]byte(exchange(`{"type":"tool_result","tool_use_id":"c","remote":true,"content_length":` + length + `}`)))
		if err != nil || r.Messages[1].Content[0].Content != RemoteResultFallback {
			t.Errorf("length %s: %v", length, err)
		}
	}
}

func TestTranscriptConsistency(t *testing.T) {
	use := Block{Type: "tool_use", ID: "c", Name: "read_file", Input: json.RawMessage(`{}`)}
	result := Block{Type: "tool_result", ToolUseID: "c"}
	user := func(b ...Block) Message { return Message{Role: "user", Content: b} }
	assistant := func(b ...Block) Message { return Message{Role: "assistant", Content: b} }
	for _, messages := range [][]Message{
		{assistant(use, use), user(result)},
		{assistant(use), user(result, result)},
		{assistant(use), assistant(result)},
		{assistant(use), user(result), assistant(use), user(result)},
	} {
		if err := validateTranscript(messages); err == nil {
			t.Error("accepted inconsistent history")
		}
	}
	otherUse := use
	otherUse.ID = "c2"
	otherResult := result
	otherResult.ToolUseID = "c2"
	if err := validateTranscript([]Message{assistant(use, otherUse), user(otherResult), user(result)}); err != nil {
		t.Fatal("parallel results:", err)
	}
	thirdUse := use
	thirdUse.ID = "c3"
	thirdResult := result
	thirdResult.ToolUseID = "c3"
	if err := validateTranscript([]Message{assistant(use, otherUse), user(otherResult), assistant(thirdUse), user(result, thirdResult)}); err != nil {
		t.Fatal("interleaved CLI groups:", err)
	}
}

func TestEstimateBounds(t *testing.T) {
	for _, chars := range []int{720000, 720003, 720004} {
		body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": strings.Repeat("x", chars)}}})
		r, err := testDecoder().Decode(body)
		if chars <= 720003 {
			if err != nil || r.EstimatedTokens != 180000 {
				t.Errorf("chars=%d: %d %v", chars, r.EstimatedTokens, err)
			}
		} else if errorCode(err) != "conversation_too_large" {
			t.Errorf("chars=%d: %v", chars, err)
		}
	}
	_, err := testDecoder().Decode([]byte(exchange(`{"type":"tool_result","tool_use_id":"c","remote":true,"content_length":9223372036854775807}`)))
	if errorCode(err) != "conversation_too_large" {
		t.Fatal(err)
	}
	r, err := testDecoder().Decode([]byte(`{"messages":[{"role":"user","content":"日本語文"}]}`))
	if err != nil || r.EstimatedTokens != 1 {
		t.Fatalf("unicode: %v %d", err, r.EstimatedTokens)
	}
}

func TestToolInputEstimateRepresentation(t *testing.T) {
	// Alternate string escaping/whitespace must not inflate the estimate.
	// Keep the original RawMessage and exact numeric lexemes for later projection.
	for _, input := range []string{`{"a":"aaaa"}`, `{ "a" : "\u0061\u0061\u0061\u0061" }`} {
		body := `{"messages":[{"role":"assistant","tool_calls":[{"id":"c","name":"suggest_command","input":` + input + `}]}]}`
		r, err := testDecoder().Decode([]byte(body))
		if err != nil || r.EstimatedTokens != 3 {
			t.Fatalf("%s: tokens %d err %v", input, r.EstimatedTokens, err)
		}
		if string(r.Messages[0].Content[0].Input) != input {
			t.Fatal("raw input changed")
		}
	}
	// Combining sequences and joined emoji count as one character each.
	tokens, err := estimateTokens([]Message{{Role: "user", Content: []Block{{Type: "text", Text: strings.Repeat("e\u0301👩‍💻", 2)}}}})
	if err != nil || tokens != 1 {
		t.Fatalf("graphemes: %d %v", tokens, err)
	}
}

func TestSessionIDs(t *testing.T) {
	for _, suffix := range []string{``, `,"session_id":null`, `,"session_id":""`, `,"session_id":"ABCDEF00-1234-1234-1234-123456789ABC"`} {
		r, err := testDecoder().Decode([]byte(`{"messages":[{"role":"user","content":"hi"}]` + suffix + `}`))
		if err != nil || !validUUID(r.SessionID) {
			t.Fatalf("session: %v", err)
		}
		if strings.Contains(suffix, "ABC") && r.SessionID != "abcdef00-1234-1234-1234-123456789abc" {
			t.Fatal(r.SessionID)
		}
	}
	for _, value := range []string{`false`, `1`, `[]`, `"123"`} {
		_, err := testDecoder().Decode([]byte(`{"messages":[{"role":"user","content":"hi"}],"session_id":` + value + `}`))
		if errorCode(err) != "invalid_request" {
			t.Errorf("%s: %v", value, err)
		}
	}
	d := testDecoder()
	d.Random = bytes.NewReader(nil)
	if _, err := d.Decode([]byte(`{"messages":[{"role":"user","content":"hi"}]}`)); errorCode(err) != "internal_error" {
		t.Fatal(err)
	}
}

func TestWorkBounds(t *testing.T) {
	for _, body := range []string{
		strings.Repeat("[", MaxJSONDepth+1) + strings.Repeat("]", MaxJSONDepth+1),
		`[` + strings.Repeat(`0,`, MaxJSONStructure) + `0]`,
		`{"messages":[` + strings.Repeat(`{"role":"user","content":""},`, MaxMessages) + `{"role":"user","content":""}]}`,
	} {
		if _, err := testDecoder().Decode([]byte(body)); errorCode(err) != "invalid_request" {
			t.Fatal(err)
		}
	}
	if _, err := testDecoder().Decode(bytes.Repeat([]byte(" "), MaxBodyBytes+1)); errorCode(err) != "request_too_large" {
		t.Fatal(err)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"messages":[{"role":"user","content":"hello"}]}`))
	f.Add([]byte(exchange(`{"type":"tool_result","tool_use_id":"c","remote":true,"content_length":null}`)))
	f.Add([]byte(`{"messages":[{"role":"assistant","tool_calls":[{"id":"c","name":"suggest_command","input":[1,null,true]}]}]}`))
	f.Add([]byte(`null`))
	data, err := os.ReadFile("../../testdata/compat/requests.json")
	if err != nil {
		f.Fatal(err)
	}
	var cases []struct{ Request json.RawMessage }
	if err := json.Unmarshal(data, &cases); err != nil {
		f.Fatal(err)
	}
	for _, tc := range cases {
		f.Add([]byte(tc.Request))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		r, err := testDecoder().Decode(body)
		if err != nil {
			return
		}
		if len(r.Messages) == 0 || len(r.Messages) > MaxMessages || r.EstimatedTokens > MaxConversationTokens || !validUUID(r.SessionID) {
			t.Fatal("invalid normalized request")
		}
		if err := validateTranscript(r.Messages); err != nil {
			t.Fatal("invalid transcript after decode")
		}
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.Type == "tool_use" && !json.Valid(b.Input) {
					t.Fatal("invalid tool JSON")
				}
				if b.Remote {
					t.Fatal("unhydrated remote")
				}
			}
		}
	})
}
