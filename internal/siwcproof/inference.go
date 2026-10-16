package siwcproof

// This is a bounded live integration probe, not the production Responses
// adapter. It never logs provider data, executes commands, or stores transcripts.
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

const apiRoot = "https://api.openai.com/v1"
const eventLimit = 1 << 20

func proofClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &inactivityConn{Conn: conn}, nil
	}
	return &http.Client{Transport: transport, Timeout: 300 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type inactivityConn struct{ net.Conn }

func (c *inactivityConn) Read(p []byte) (int, error) {
	if err := c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

type CatalogModel struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Visibility  string `json:"visibility"`
}

type CatalogProof struct {
	Models             []CatalogModel `json:"-"`
	Count              int            `json:"model_count"`
	HasModelsArray     bool           `json:"has_models_array"`
	HasAPIKeyDataArray bool           `json:"has_data_array"`
}

func ReadCatalog(ctx context.Context, token string) (CatalogProof, error) {
	return readCatalog(ctx, proofClient(), apiRoot, token)
}

func readCatalog(ctx context.Context, client *http.Client, root, token string) (CatalogProof, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", root+"/models", nil)
	if err != nil {
		return CatalogProof{}, errors.New("catalog request creation failed")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := client.Do(req)
	if err != nil {
		return CatalogProof{}, errors.New("catalog transport failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return CatalogProof{}, fmt.Errorf("catalog HTTP status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
	if err != nil || len(body) > 4<<20 {
		return CatalogProof{}, errors.New("catalog response exceeds bound or read failed")
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(body, &shape) != nil {
		return CatalogProof{}, errors.New("catalog JSON invalid")
	}
	var models []CatalogModel
	raw, ok := shape["models"]
	if !ok || len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &models) != nil {
		return CatalogProof{}, errors.New("catalog does not have documented models array")
	}
	_, hasData := shape["data"]
	return CatalogProof{Models: models, Count: len(models), HasModelsArray: true, HasAPIKeyDataArray: hasData}, nil
}

// SelectModel never assumes a slug absent from the actual account catalog.
func SelectModel(c CatalogProof, requested string) (string, error) {
	if requested != "" {
		for _, m := range c.Models {
			if m.Slug == requested && m.Visibility == "list" {
				return requested, nil
			}
		}
		return "", errors.New("requested model is not visible in account catalog")
	}
	for _, preferred := range []string{"gpt-6.1-sol", "gpt-6-sol", "gpt-5.5", "gpt-5"} {
		for _, m := range c.Models {
			if m.Slug == preferred && m.Visibility == "list" {
				return m.Slug, nil
			}
		}
	}
	return "", errors.New("no preferred text model found; select a visible model explicitly")
}

type InferenceProof struct {
	Model                    string `json:"model"`
	TextCompleted            bool   `json:"text_completed"`
	ToolCompleted            bool   `json:"tool_completed"`
	ContinuationCompleted    bool   `json:"continuation_completed"`
	CallIDCorrelated         bool   `json:"call_id_correlated"`
	ReasoningItemsPresent    bool   `json:"reasoning_items_present"`
	ReplayWithoutOpaqueState bool   `json:"replay_without_opaque_state"`
}

type proofItem struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content,omitempty"`
}

type completedResponse struct {
	Status string      `json:"status"`
	Output []proofItem `json:"output"`
}

type streamResult struct {
	response     completedResponse
	text         string
	sawTextDelta bool
}

func ProveInference(ctx context.Context, token, model string) (InferenceProof, error) {
	return proveInference(ctx, proofClient(), apiRoot, token, model)
}

func proveInference(ctx context.Context, client *http.Client, root, token, model string) (InferenceProof, error) {
	report := InferenceProof{Model: model}
	textInput := []any{map[string]any{"role": "user", "content": "Say exactly: Hello, world!"}}
	result, err := generate(ctx, client, root, token, map[string]any{"model": model, "input": textInput, "store": false, "stream": true})
	if err != nil {
		return report, err
	}
	if !result.sawTextDelta || !strings.Contains(result.text, "Hello, world!") {
		return report, errors.New("text proof did not stream expected greeting")
	}
	report.TextCompleted = true
	tools := []any{map[string]any{"type": "namespace", "name": "proof", "description": "Harmless synthetic integration test; no machine access.", "tools": []any{map[string]any{"type": "function", "name": "echo_marker", "description": "Return the deterministic test marker. Takes no arguments.", "strict": true, "parameters": map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}, "additionalProperties": false}}}}}
	toolInput := []any{map[string]any{"role": "user", "content": "Call proof.echo_marker once, with no arguments. After receiving its output, repeat the exact marker returned by the tool and nothing else."}}
	result, err = generate(ctx, client, root, token, map[string]any{"model": model, "input": toolInput, "tools": tools, "tool_choice": "required", "store": false, "stream": true})
	if err != nil {
		return report, err
	}
	var call *proofItem
	// Intentionally test stateless replay WITHOUT opaque reasoning state. This
	// is a validation experiment, not permission for production to drop state.
	for i := range result.response.Output {
		item := &result.response.Output[i]
		switch item.Type {
		case "reasoning":
			report.ReasoningItemsPresent = true
		case "function_call":
			if call != nil || (item.Name != "echo_marker" && item.Name != "proof.echo_marker") || (item.Namespace != "" && item.Namespace != "proof") || item.CallID == "" || item.ID == "" || item.CallID == item.ID || len(item.Arguments) > 256<<10 {
				return report, errors.New("unexpected synthetic function call")
			}
			var args map[string]json.RawMessage
			if json.Unmarshal([]byte(item.Arguments), &args) != nil || args == nil || len(args) != 0 {
				return report, errors.New("synthetic function arguments invalid")
			}
			call = item
		case "message":
			// Preserve assistant prose as semantic messages without provider IDs.
			var content strings.Builder
			for _, c := range item.Content {
				if c.Type == "output_text" {
					content.WriteString(c.Text)
				}
			}
			if content.Len() > 0 {
				toolInput = append(toolInput, map[string]any{"role": "assistant", "content": content.String()})
			}
		default:
			return report, errors.New("unsupported output item in tool proof")
		}
	}
	if call == nil {
		return report, errors.New("synthetic function was not called")
	}
	report.ToolCompleted = true
	replayCall := map[string]any{"type": "function_call", "call_id": call.CallID, "name": call.Name, "arguments": call.Arguments}
	if call.Namespace != "" {
		replayCall["namespace"] = call.Namespace
	}
	toolInput = append(toolInput, replayCall, map[string]any{"type": "function_call_output", "call_id": call.CallID, "output": "SIWC_PROOF_OK_42"})
	result, err = generate(ctx, client, root, token, map[string]any{"model": model, "input": toolInput, "tools": tools, "tool_choice": "none", "store": false, "stream": true})
	if err != nil {
		return report, err
	}
	for _, item := range result.response.Output {
		if item.Type != "message" && item.Type != "reasoning" {
			return report, errors.New("unexpected continuation output")
		}
	}
	if !strings.Contains(result.text, "SIWC_PROOF_OK_42") {
		return report, errors.New("continuation did not use synthetic output")
	}
	report.ContinuationCompleted = true
	report.CallIDCorrelated = true
	report.ReplayWithoutOpaqueState = true
	return report, nil
}

func generate(ctx context.Context, client *http.Client, root, token string, body map[string]any) (streamResult, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return streamResult{}, errors.New("cannot encode proof request")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", root+"/responses", bytes.NewReader(encoded))
	if err != nil {
		return streamResult{}, errors.New("cannot create proof request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	res, err := client.Do(req)
	if err != nil {
		return streamResult{}, errors.New("responses transport failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return streamResult{}, fmt.Errorf("responses HTTP status %d", res.StatusCode)
	}
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		return streamResult{}, errors.New("responses content type is not SSE")
	}
	return parseProofStream(res.Body)
}

func parseProofStream(input io.Reader) (streamResult, error) {
	result := streamResult{}
	reader := bufio.NewReaderSize(io.LimitReader(input, 16<<20), 4096)
	var line, event []byte
	textBytes := 0
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > eventLimit {
			return result, errors.New("SSE line exceeds bound")
		}
		line = append(line, fragment...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return result, errors.New("SSE ended before successful completion")
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
					return result, errors.New("SSE event JSON invalid")
				}
				switch frame.Type {
				case "response.output_text.delta":
					textBytes += len(frame.Delta)
					if textBytes > eventLimit {
						return result, errors.New("proof text exceeds bound")
					}
					result.text += frame.Delta
					result.sawTextDelta = true
				case "response.completed":
					if frame.Response.Status != "completed" || len(frame.Response.Output) > 32 {
						return result, errors.New("invalid completion snapshot")
					}
					result.response = frame.Response
					return result, nil
				case "response.failed", "response.incomplete", "error":
					return result, errors.New("responses reported unsuccessful terminal event")
				}
			}
			event = event[:0]
		} else if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimPrefix(line, []byte("data:"))
			data = bytes.TrimPrefix(data, []byte(" "))
			if len(event)+len(data)+1 > eventLimit {
				return result, errors.New("SSE event exceeds bound")
			}
			event = append(event, data...)
			event = append(event, '\n')
		}
		line = line[:0]
	}
}
