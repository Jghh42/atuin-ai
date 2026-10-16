// Package tools defines the model-facing local-tool contract. It never executes
// tools: the Atuin CLI owns local execution and permissions.
package tools

import "encoding/json"

// Definition is backend-neutral. Responses projection must wrap these fields
// in its own provider-specific tool format rather than assuming this is wire JSON.
type Definition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Call is a completed provider function call. ID is Responses call_id, not an
// output-item ID. Input is kept as raw JSON to preserve number precision.
type Call struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type BatchKind string

const (
	TextOnly   BatchKind = "text"
	LocalTools BatchKind = "local_tools"
	Suggestion BatchKind = "suggestion"
)

// Batch is emitted only after successful provider completion and validation of
// every call. ValidateBatch returns no partial batch on any failure.
type Batch struct {
	Kind  BatchKind
	Calls []Call
}
