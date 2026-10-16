package codexproof

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatelessProjectionAndSyntheticContinuation(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Header.Get("chatgpt-account-id") != "synthetic-account" || r.Header.Get("originator") != "atuin-ai" {
			t.Error("invalid credentials/header binding")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid request JSON")
		}
		if string(body["store"]) != "false" || string(body["stream"]) != "true" || body["previous_response_id"] != nil || body["instructions"] == nil {
			t.Error("invalid stateless flags")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) { data, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", data) }
		var output []any
		if count == 2 {
			var tools []map[string]any
			if json.Unmarshal(body["tools"], &tools) != nil || len(tools) != 1 || tools[0]["type"] != "function" || tools[0]["name"] != "echo_marker" {
				t.Error("invalid Codex tool declaration")
			}
			output = []any{map[string]any{"type": "reasoning"}, map[string]any{"type": "function_call", "id": "item_1", "call_id": "call_1", "name": "echo_marker", "arguments": "{}"}}
		} else {
			text := "Hello, world!"
			if count == 3 {
				var input []map[string]any
				if json.Unmarshal(body["input"], &input) != nil || len(input) != 3 {
					t.Error("invalid continuation")
				} else if input[1]["call_id"] != "call_1" || input[2]["call_id"] != "call_1" || input[1]["id"] != nil {
					t.Error("incorrect call-ID projection")
				}
				text = "CODEX_PROOF_OK_42"
			}
			emit(map[string]any{"type": "response.output_text.delta", "delta": text})
			output = []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": text}}}}
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": output}})
	}))
	defer server.Close()
	report, err := proveInference(context.Background(), server.Client(), server.URL, "synthetic-token", "synthetic-account", "test-model")
	if err != nil || count != 3 || !report.TextCompleted || !report.ToolCompleted || !report.ContinuationCompleted || !report.CallIDCorrelated || !report.ReasoningItemsPresent || !report.ReplayWithoutOpaqueState {
		t.Fatalf("proof failed %+v err=%v count=%d", report, err, count)
	}
}

func TestMalformedToolsNeverReachContinuation(t *testing.T) {
	for _, kind := range []string{"duplicate", "missing_call_id", "invalid_arguments", "unknown_tool", "item_id_as_call_id"} {
		t.Run(kind, func(t *testing.T) {
			count := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				w.Header().Set("Content-Type", "text/event-stream")
				emit := func(v any) { data, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", data) }
				output := []any{}
				if count == 1 {
					emit(map[string]any{"type": "response.output_text.delta", "delta": "Hello, world!"})
					output = []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "Hello, world!"}}}}
				} else {
					call := map[string]any{"type": "function_call", "id": "item_1", "call_id": "call_1", "name": "echo_marker", "arguments": "{}"}
					switch kind {
					case "missing_call_id":
						delete(call, "call_id")
					case "invalid_arguments":
						call["arguments"] = `{"command":"never run"}`
					case "unknown_tool":
						call["name"] = "execute_shell_command"
					case "item_id_as_call_id":
						call["call_id"] = "item_1"
					}
					output = append(output, call)
					if kind == "duplicate" {
						output = append(output, call)
					}
				}
				emit(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": output}})
			}))
			defer server.Close()
			report, err := proveInference(context.Background(), server.Client(), server.URL, "synthetic-token", "synthetic-account", "test-model")
			if err == nil || report.ToolCompleted || report.ContinuationCompleted || count != 2 {
				t.Fatal("invalid batch reached continuation")
			}
		})
	}
}

func TestCodexFramingNotMimeHeaderDeterminesStream(t *testing.T) {
	for _, media := range []string{"text/plain", "application/json", "text/event-stream"} {
		t.Run(media, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", media)
				fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n")
			}))
			defer server.Close()
			if _, err := generate(context.Background(), server.Client(), server.URL, "synthetic-token", "synthetic-account", map[string]any{}); err != nil {
				t.Fatal(err)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"error":"never-print-this-private-diagnostic"}`)
	}))
	defer server.Close()
	if _, err := generate(context.Background(), server.Client(), server.URL, "synthetic-token", "synthetic-account", map[string]any{}); err == nil || strings.Contains(err.Error(), "private-diagnostic") {
		t.Fatal("non-SSE response accepted or leaked")
	}
}

func TestStreamRequiresBoundedExplicitSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, stream string
		success      bool
	}{
		{"completed", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n", true},
		{"EOF", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n", false},
		{"Codex done", "data: {\"type\":\"response.done\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n", true},
		{"Codex done incomplete", "data: {\"type\":\"response.done\",\"response\":{\"status\":\"incomplete\",\"output\":[]}}\n\n", false},
		{"incomplete", "data: {\"type\":\"response.incomplete\"}\n\n", false},
		{"failed", "data: {\"type\":\"response.failed\"}\n\n", false},
		{"malformed", "data: {bad}\n\n", false},
		{"snapshot mismatch", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n", false},
		{"oversized", strings.Repeat("x", maxEventBytes+1) + "\n", false},
		{"CRLF multiline", ": heartbeat\r\ndata: {\"type\":\"response.completed\",\r\ndata: \"response\":{\"status\":\"completed\",\"output\":[]}}\r\n\r\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseStream(strings.NewReader(tc.stream))
			if (err == nil) != tc.success {
				t.Fatalf("success=%v error=%v", tc.success, err)
			}
		})
	}
}
