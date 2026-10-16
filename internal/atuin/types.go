// Package atuin decodes and normalizes the bounded, stateless CLI protocol.
package atuin

import "encoding/json"

const (
	MaxBodyBytes          = 4 << 20
	MaxConversationTokens = 180_000
	RemoteResultFallback  = "[Tool result no longer available]"
)

// Request contains only normalized data; no provider state or credentials.
type Request struct {
	Messages        []Message
	SessionID       string
	InvocationID    *string
	Context         Context
	Config          Config
	EstimatedTokens int
}

// Message always uses blocks, including messages originally encoded as strings.
type Message struct {
	Role    string
	Content []Block
}

// Block is a tagged union selected by Type: text, tool_use, or tool_result.
// Input retains exact JSON number precision. Remote results are replaced by
// fallback text after estimating the original declared length.
type Block struct {
	Type          string
	Text          string
	ID            string
	Name          string
	Input         json.RawMessage
	ToolUseID     string
	Content       string
	IsError       bool
	Remote        bool
	ContentLength *int64
}

type Context struct {
	OS, Distro, Shell, PreferredLanguage, PWD, LastCommand *string
}

type Config struct {
	Capabilities   Capabilities
	RunPreference  string
	ModelAlias     string
	ProviderModel  string
	UserContexts   []UserContext
	Skills         []Skill
	SkillsOverflow *string
	// PromptFn is always empty: standalone mode disables custom prompt selection.
	PromptFn string
}

type UserContext struct{ Path, Data string }
type Skill struct{ Name, Description string }

// Capabilities contains only recognized v0.4.0 markers.
type Capabilities map[string]bool

func (c Capabilities) Has(name string) bool { return c[name] }

// ModelCatalog is an immutable alias snapshot supplied by server configuration.
// Default must resolve in Models. Unknown request aliases use Default.
type ModelCatalog struct {
	Default string
	Models  map[string]string
}

// Error is a safe, fixed pre-stream protocol error, never containing user data.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Message }
func invalid() error           { return &Error{"invalid_request", "Invalid chat request"} }
