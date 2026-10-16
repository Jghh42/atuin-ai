package codexproof

// This probe tests a single isolated Codex token session. It is not the
// production provider adapter and never executes model-selected code.
import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const responsesEndpoint = "https://chatgpt.com/backend-api/codex/responses"
const maxEventBytes = 1 << 20

func inferenceClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.MaxResponseHeaderBytes = 32 << 10
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &deadlineConn{Conn: conn}, nil
	}
	return &http.Client{Transport: transport, Timeout: 300 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type deadlineConn struct{ net.Conn }

func (c *deadlineConn) Read(p []byte) (int, error) {
	if err := c.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

type InferenceReport struct {
	Model                    string `json:"model"`
	TextCompleted            bool   `json:"text_completed"`
	ToolCompleted            bool   `json:"tool_completed"`
	ContinuationCompleted    bool   `json:"continuation_completed"`
	CallIDCorrelated         bool   `json:"call_id_correlated"`
	ReasoningItemsPresent    bool   `json:"reasoning_items_present"`
	ReplayWithoutOpaqueState bool   `json:"replay_without_opaque_state"`
}

type outputItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}
type completedResponse struct {
	Status string       `json:"status"`
	Output []outputItem `json:"output"`
}
type streamResult struct {
	response completedResponse
	text     string
	sawDelta bool
}

// ProveInference makes exactly three generations: greeting, synthetic tool,
// full semantic-history continuation. Raw provider data never enters the report.
func ProveInference(ctx context.Context, token, accountID, model string) (InferenceReport, error) {
	return proveInference(ctx, inferenceClient(), responsesEndpoint, token, accountID, model)
}
func proveInference(ctx context.Context, client *http.Client, endpoint, token, accountID, model string) (InferenceReport, error) {
	report := InferenceReport{Model: model}
	if token == "" || accountID == "" || model == "" {
		return report, errors.New("proof requires an isolated credential and model")
	}
	base := func(input []any) map[string]any {
		return map[string]any{
			"model": model, "input": input, "instructions": "You are a helpful assistant. Follow the user's synthetic integration test exactly.",
			"store": false, "stream": true, "include": []string{"reasoning.encrypted_content"},
		}
	}
	result, err := generate(ctx, client, endpoint, token, accountID, base([]any{map[string]any{"role": "user", "content": "Say exactly: Hello, world!"}}))
	if err != nil {
		return report, err
	}
	if !result.sawDelta || strings.TrimSpace(result.text) != "Hello, world!" {
		return report, errors.New("greeting proof did not stream expected text")
	}
	report.TextCompleted = true
	tools := []any{map[string]any{"type": "function", "name": "echo_marker", "description": "Harmless synthetic test. Return a deterministic marker. No arguments or machine access.", "parameters": map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}, "additionalProperties": false}, "strict": true}}
	input := []any{map[string]any{"role": "user", "content": "Call echo_marker once with no arguments. After receiving its output, repeat the exact returned marker and nothing else."}}
	body := base(input)
	body["tools"] = tools
	body["tool_choice"] = "required"
	result, err = generate(ctx, client, endpoint, token, accountID, body)
	if err != nil {
		return report, err
	}
	var call *outputItem
	// This experiment explicitly omits opaque state, testing whether semantic
	// Atuin history alone suffices. Production must not silently discard state.
	for i := range result.response.Output {
		item := &result.response.Output[i]
		switch item.Type {
		case "reasoning":
			report.ReasoningItemsPresent = true
		case "function_call":
			if call != nil || item.Name != "echo_marker" || item.CallID == "" || item.ID == "" || item.CallID == item.ID || len(item.Arguments) > 256<<10 {
				return report, errors.New("unexpected synthetic function call")
			}
			var args map[string]json.RawMessage
			if json.Unmarshal([]byte(item.Arguments), &args) != nil || args == nil || len(args) != 0 {
				return report, errors.New("synthetic arguments invalid")
			}
			call = item
		case "message":
			var text strings.Builder
			for _, part := range item.Content {
				if part.Type == "output_text" {
					text.WriteString(part.Text)
				}
			}
			if text.Len() > 0 {
				input = append(input, map[string]any{"role": "assistant", "content": text.String()})
			}
		default:
			return report, errors.New("unsupported tool-proof output item")
		}
	}
	if call == nil {
		return report, errors.New("synthetic function was not called")
	}
	report.ToolCompleted = true
	input = append(input, map[string]any{"type": "function_call", "call_id": call.CallID, "name": call.Name, "arguments": call.Arguments}, map[string]any{"type": "function_call_output", "call_id": call.CallID, "output": "CODEX_PROOF_OK_42"})
	body = base(input)
	body["tools"] = tools
	body["tool_choice"] = "none"
	result, err = generate(ctx, client, endpoint, token, accountID, body)
	if err != nil {
		return report, err
	}
	for _, item := range result.response.Output {
		if item.Type != "message" && item.Type != "reasoning" {
			return report, errors.New("unexpected continuation output")
		}
	}
	if strings.TrimSpace(result.text) != "CODEX_PROOF_OK_42" {
		return report, errors.New("continuation did not use synthetic output")
	}
	report.ContinuationCompleted = true
	report.CallIDCorrelated = true
	report.ReplayWithoutOpaqueState = true
	return report, nil
}

func generate(ctx context.Context, client *http.Client, endpoint, token, accountID string, body map[string]any) (streamResult, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return streamResult{}, errors.New("proof request encoding failed")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(encoded))
	if err != nil {
		return streamResult{}, errors.New("proof request creation failed")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("originator", "atuin-ai")
	req.Header.Set("User-Agent", "atuin-ai-codex-proof/0.1")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	res, err := client.Do(req)
	if err != nil {
		return streamResult{}, errors.New("codex inference transport failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return streamResult{}, fmt.Errorf("codex inference HTTP status %d", res.StatusCode)
	}
	// Pi's Codex parser consumes SSE framing without relying on the MIME
	// header. Some compatible routes use a nonstandard media type. Require
	// recognizable framing and an explicit valid completion, not just HTTP200.
	reader := bufio.NewReader(res.Body)
	prefix, _ := reader.Peek(32)
	trimmed := bytes.TrimSpace(prefix)
	if !bytes.HasPrefix(trimmed, []byte("data:")) && !bytes.HasPrefix(trimmed, []byte("event:")) && !bytes.HasPrefix(trimmed, []byte(":")) {
		media := "other"
		header := strings.ToLower(strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0]))
		switch header {
		case "application/json":
			media = "json"
		case "text/html":
			media = "html"
		case "text/plain":
			media = "text"
		case "":
			media = "missing"
		}
		return streamResult{}, fmt.Errorf("codex inference response lacks SSE framing (media class: %s)", media)
	}
	return parseStream(reader)
}

func parseStream(input io.Reader) (streamResult, error) {
	result := streamResult{}
	reader := bufio.NewReaderSize(io.LimitReader(input, 16<<20), 4096)
	var line, event []byte
	var text strings.Builder
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxEventBytes {
			return result, errors.New("SSE line exceeds bound")
		}
		line = append(line, fragment...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return result, errors.New("SSE ended without completed response")
		}
		line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
		if len(line) == 0 {
			if len(event) > 0 {
				var frame struct {
					Type     string            `json:"type"`
					Delta    string            `json:"delta"`
					Response completedResponse `json:"response"`
				}
				if json.Unmarshal(event, &frame) != nil {
					return result, errors.New("invalid SSE JSON")
				}
				switch frame.Type {
				case "response.output_text.delta":
					if text.Len()+len(frame.Delta) > maxEventBytes {
						return result, errors.New("proof text exceeds bound")
					}
					text.WriteString(frame.Delta)
					result.sawDelta = true
				case "response.completed", "response.done":
					if frame.Response.Status != "completed" || len(frame.Response.Output) > 32 {
						return result, errors.New("invalid completion snapshot")
					}
					var final strings.Builder
					for _, item := range frame.Response.Output {
						if item.Type != "message" {
							continue
						}
						for _, part := range item.Content {
							if part.Type != "output_text" {
								return result, errors.New("unsupported final message content")
							}
							if final.Len()+len(part.Text) > maxEventBytes {
								return result, errors.New("final text exceeds bound")
							}
							final.WriteString(part.Text)
						}
					}
					if final.String() != text.String() {
						return result, errors.New("completed text does not match streamed text")
					}
					result.response = frame.Response
					result.text = text.String()
					return result, nil
				case "response.failed", "response.incomplete", "error":
					return result, errors.New("codex inference unsuccessful terminal event")
				}
			}
			event = event[:0]
		} else if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" "))
			if len(event)+len(data)+1 > maxEventBytes {
				return result, errors.New("SSE event exceeds bound")
			}
			event = append(event, data...)
			event = append(event, '\n')
		}
		line = line[:0]
	}
}
