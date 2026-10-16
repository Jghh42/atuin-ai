package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/prompt"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

const ResponsesURL = "https://chatgpt.com/backend-api/codex/responses"

var (
	ErrRequest     = errors.New("invalid Codex generation request")
	ErrCredentials = errors.New("codex credentials unavailable")
	ErrTransport   = errors.New("codex generation transport failed")
	ErrResponse    = errors.New("codex generation rejected")
)

type functionDefinition struct {
	Type string `json:"type"`
	tools.Definition
	// Pi's Codex adapter uses strict:null, not public Responses strict:true.
	Strict any `json:"strict"`
}

type requestBody struct {
	Model        string `json:"model"`
	Store        bool   `json:"store"`
	Stream       bool   `json:"stream"`
	Instructions string `json:"instructions"`
	Input        []any  `json:"input"`
	Text         struct {
		Verbosity string `json:"verbosity"`
	} `json:"text"`
	Include           []string             `json:"include"`
	ToolChoice        string               `json:"tool_choice"`
	ParallelToolCalls bool                 `json:"parallel_tool_calls"`
	Tools             []functionDefinition `json:"tools"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type functionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type functionOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// Project accepts the decoder's normalized request, not arbitrary wire input.
// TurnContext is a separate first user input item on EVERY request, followed by
// all history in original block order. It is data, not part of instructions.
// No opaque state or item IDs are invented. Model eligibility is owned by the
// catalog; this projection does not expand the replay-proven allowlist.
func Project(req provider.Request, now time.Time) ([]byte, error) {
	r := req.Conversation
	if !bounded(r) {
		return nil, ErrRequest
	}
	body := requestBody{
		Model: r.Config.ProviderModel, Stream: true,
		Instructions: prompt.Build(r.Context, r.Config),
		Input:        []any{message{"user", prompt.TurnContext(r.Context, now)}},
		Include:      []string{"reasoning.encrypted_content"}, ToolChoice: "auto", ParallelToolCalls: true,
	}
	body.Text.Verbosity = "low"
	for _, d := range tools.Definitions(r.Config.Capabilities) {
		body.Tools = append(body.Tools, functionDefinition{Type: "function", Definition: d})
	}
	seen, pending := map[string]bool{}, map[string]bool{}
	for _, m := range r.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			return nil, ErrRequest
		}
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				body.Input = append(body.Input, message{m.Role, b.Text})
			case "tool_use":
				if m.Role != "assistant" || b.ID == "" || b.Name == "" || seen[b.ID] || !json.Valid(b.Input) {
					return nil, ErrRequest
				}
				seen[b.ID] = true
				if b.Name == "suggest_command" {
					// Atuin suggestions are terminal answers, not executable
					// calls, and legitimately have no result. Replay as text
					// instead of a dangling Responses call or fabricated output.
					body.Input = append(body.Input, message{"assistant", "Terminal command suggestion (suggest_command; suggestion only, not evidence of execution). Original arguments JSON: " + string(b.Input)})
					continue
				}
				pending[b.ID] = true
				body.Input = append(body.Input, functionCall{"function_call", b.ID, b.Name, string(b.Input)})
			case "tool_result":
				if m.Role != "user" || !pending[b.ToolUseID] {
					return nil, ErrRequest
				}
				delete(pending, b.ToolUseID)
				output := b.Content
				if b.IsError {
					output = "Tool execution failed: " + output
				}
				body.Input = append(body.Input, functionOutput{"function_call_output", b.ToolUseID, output})
			default:
				return nil, ErrRequest
			}
		}
	}
	if len(pending) != 0 {
		return nil, ErrRequest
	}
	encoded, err := json.Marshal(body)
	// JSON escaping can expand input; bound the actual outbound allocation too.
	if err != nil || len(encoded) > 8*atuin.MaxBodyBytes {
		return nil, ErrRequest
	}
	return encoded, nil
}

// Bound work before rendering/marshaling, including calls from internal users
// accidentally bypassing the decoder. Decoder remains responsible for protocol
// normalization, grapheme/token limits and JSON structural validation.
func bounded(r atuin.Request) bool {
	if len(r.Messages) == 0 || len(r.Messages) > atuin.MaxMessages || r.Config.ProviderModel == "" || len(r.Config.ProviderModel) > 256 {
		return false
	}
	budget, blocks := atuin.MaxBodyBytes, 0
	take := func(s string) { budget -= len(s) }
	for _, p := range []*string{r.Context.OS, r.Context.Distro, r.Context.Shell, r.Context.PreferredLanguage, r.Context.PWD, r.Context.LastCommand, r.Config.SkillsOverflow} {
		if p != nil {
			take(*p)
		}
	}
	if len(r.Config.UserContexts) > atuin.MaxBlocks || len(r.Config.Skills) > atuin.MaxBlocks {
		return false
	}
	for _, c := range r.Config.UserContexts {
		take(c.Path)
		take(c.Data)
	}
	for _, s := range r.Config.Skills {
		take(s.Name)
		take(s.Description)
	}
	for _, m := range r.Messages {
		blocks += len(m.Content)
		if blocks > atuin.MaxBlocks {
			return false
		}
		calls := 0
		for _, b := range m.Content {
			if b.Type == "tool_use" {
				calls++
			}
			if calls > config.MaxCallsPerResponse {
				return false
			}
			if len(b.Input) > config.MaxCallArgumentsBytes {
				return false
			}
			take(b.Text)
			take(b.ID)
			take(b.Name)
			budget -= len(b.Input)
			take(b.ToolUseID)
			take(b.Content)
		}
	}
	return budget >= 0
}

// RequestClient implements Provider.Stream with one native HTTP generation.
// Open alone only returns headers: HTTP 200 is not generation success. Direct
// Open callers own Body.Close and must keep ctx alive through consumption;
// Stream owns bounded parsing, explicit completion and upstream cancellation.
// URL/Transport injection is for local deterministic tests.
type RequestClient struct {
	Credentials credentials.Source
	Transport   http.RoundTripper
	URL         string
	Now         func() time.Time
}

func (c RequestClient) Open(ctx context.Context, req provider.Request) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	body, err := Project(req, now)
	if err != nil {
		return nil, err
	}
	if c.Credentials == nil {
		return nil, ErrCredentials
	}
	credential, err := c.Credentials.Current(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || credential.Identity.Provider != "codex" || credential.Identity.Account == "" || credential.AccessToken == "" {
		return nil, ErrCredentials
	}
	url := c.URL
	if url == "" {
		url = ResponsesURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, ErrRequest
	}
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	request.Header.Set("chatgpt-account-id", credential.Identity.Account)
	request.Header.Set("originator", "atuin-ai")
	request.Header.Set("User-Agent", "atuin-ai/phase4")
	request.Header.Set("OpenAI-Beta", "responses=experimental")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	client := http.Client{Timeout: config.GenerationTimeout, Transport: c.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrTransport
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, ErrResponse
	}
	return response, nil
}
