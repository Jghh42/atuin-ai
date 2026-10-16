package httpserver

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/config"
)

func TestChatProtocolBoundary(t *testing.T) {
	base, _, _ := start(t, config.Default(), nil)
	for _, tc := range []struct {
		name, body, code string
		status           int
	}{
		{"malformed", `{"messages":`, "invalid_request", 400},
		{"session", `{"messages":[{"role":"user","content":"secret message"}],"session_id":"secret session"}`, "invalid_request", 400},
		{"oversized conversation", `{"messages":[{"role":"user","content":"` + strings.Repeat("a", 720004) + `"}]}`, "conversation_too_large", 400},
		{"advisory fallback", `{"messages":[{"role":"user","content":"hello"}],"config":123,"context":false}`, "upstream_unavailable", 503},
		{"parallel interleaved", `{"messages":[{"role":"assistant","tool_calls":[{"id":"a","name":"read_file","input":{}},{"id":"b","name":"read_file","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","remote":true}]},{"role":"assistant","tool_calls":[{"id":"c","name":"read_file","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"done"},{"type":"tool_result","tool_use_id":"c","content":"done"}]}]}`, "upstream_unavailable", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := request(t, "POST", base+"/api/cli/chat", strings.NewReader(tc.body), tc.status)
			var got struct{ Error string }
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.Error != tc.code {
				t.Fatalf("%s", data)
			}
			if bytes.Contains(data, []byte("secret")) {
				t.Fatal("request data leaked")
			}
		})
	}
}
