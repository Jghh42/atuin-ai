// Package catalog projects configuration into the public /api/cli/models JSON
// shape. It performs no provider discovery or model-availability claims.
package catalog

import "github.com/Jghh42/atuin-ai/internal/config"

// Model is a public model entry. Upstream IDs are intentionally absent.
type Model struct {
	Alias       string `json:"alias"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Response is the JSON body for GET /api/cli/models.
type Response struct {
	Default string  `json:"default"`
	Models  []Model `json:"models"`
}

// New validates c and returns an independent public snapshot in configuration
// order. When DefaultModel is empty, the first alias is selected. Callers may
// serialize the result with encoding/json; no HTTP or upstream work is done.
func New(c config.Config) (Response, error) {
	if err := c.Validate(); err != nil {
		return Response{}, err
	}
	r := Response{Default: c.DefaultModel, Models: make([]Model, len(c.Models))}
	if r.Default == "" {
		r.Default = c.Models[0].Alias
	}
	for i, m := range c.Models {
		r.Models[i] = Model{Alias: m.Alias, Name: m.Name, Description: m.Description}
	}
	return r, nil
}
