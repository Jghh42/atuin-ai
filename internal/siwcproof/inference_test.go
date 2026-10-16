package siwcproof

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProofStream(t *testing.T) {
	for _, tc := range []struct {
		name, stream string
		ok           bool
	}{
		{"completed", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n", true},
		{"EOF", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n", false},
		{"failed", "data: {\"type\":\"response.failed\"}\n\n", false},
		{"incomplete", "data: {\"type\":\"response.incomplete\"}\n\n", false},
		{"bad snapshot", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\"}}\n\n", false},
		{"malformed", "data: {oops}\n\n", false},
		{"oversized", strings.Repeat("x", eventLimit+1) + "\n", false},
		{"CRLF multiline", ": heartbeat\r\ndata: {\"type\":\"response.completed\",\r\ndata: \"response\":{\"status\":\"completed\",\"output\":[]}}\r\n\r\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseProofStream(strings.NewReader(tc.stream))
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v err=%v", tc.ok, err)
			}
		})
	}
}

func TestInferenceProjectionAndReplay(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("missing token")
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("bad request")
		}
		if string(body["store"]) != "false" || string(body["stream"]) != "true" || body["previous_response_id"] != nil {
			t.Error("invalid stateless flags")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(data any) { b, _ := json.Marshal(data); fmt.Fprintf(w, "data: %s\n\n", b) }
		text := "Hello, world!"
		output := []any{map[string]any{"type": "message"}}
		if count == 2 {
			var tools []struct {
				Type string `json:"type"`
				Name string `json:"name"`
			}
			if json.Unmarshal(body["tools"], &tools) != nil || len(tools) != 1 || tools[0].Type != "namespace" || tools[0].Name != "proof" {
				t.Error("missing namespace")
			}
			output = []any{map[string]any{"type": "reasoning"}, map[string]any{"type": "function_call", "id": "item_1", "call_id": "call_1", "namespace": "proof", "name": "echo_marker", "arguments": "{}"}}
		} else {
			if count == 3 {
				var input []map[string]any
				if json.Unmarshal(body["input"], &input) != nil || len(input) != 3 {
					t.Error("bad continuation input")
				} else {
					if input[1]["call_id"] != "call_1" || input[2]["call_id"] != "call_1" || input[1]["id"] != nil || input[1]["namespace"] != "proof" {
						t.Error("call ID projection invalid")
					}
				}
				text = "SIWC_PROOF_OK_42"
			}
			emit(map[string]any{"type": "response.output_text.delta", "delta": text})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": output}})
	}))
	defer server.Close()
	report, err := proveInference(context.Background(), server.Client(), server.URL, "synthetic", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 || !report.TextCompleted || !report.ToolCompleted || !report.CallIDCorrelated || !report.ContinuationCompleted || !report.ReasoningItemsPresent || !report.ReplayWithoutOpaqueState {
		t.Fatalf("bad proof report: %+v", report)
	}
}

func TestRejectMalformedToolBatchBeforeContinuation(t *testing.T) {
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
				} else {
					call := map[string]any{"type": "function_call", "id": "item_1", "call_id": "call_1", "name": "echo_marker", "arguments": "{}"}
					switch kind {
					case "missing_call_id":
						delete(call, "call_id")
					case "invalid_arguments":
						call["arguments"] = `{"command":"never execute"}`
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
			report, err := proveInference(context.Background(), server.Client(), server.URL, "synthetic", "test-model")
			if err == nil || report.ToolCompleted || report.ContinuationCompleted || count != 2 {
				t.Fatalf("invalid batch reached continuation: %+v err=%v requests=%d", report, err, count)
			}
		})
	}
}

func TestCatalogShapeAndSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"models":[{"slug":"gpt-6.1-sol","display_name":"Test","visibility":"list"}]}`)
	}))
	defer server.Close()
	c, err := readCatalog(context.Background(), server.Client(), server.URL, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if c.Count != 1 || !c.HasModelsArray || c.HasAPIKeyDataArray {
		t.Fatalf("bad catalog: %+v", c)
	}
	if model, err := SelectModel(c, ""); err != nil || model != "gpt-6.1-sol" {
		t.Fatalf("selection failed %s %v", model, err)
	}
	if _, err := SelectModel(c, "absent"); err == nil {
		t.Fatal("selected absent model")
	}
	c.Models[0].Visibility = "hide"
	if _, err := SelectModel(c, ""); err == nil {
		t.Fatal("selected hidden model")
	}
}
