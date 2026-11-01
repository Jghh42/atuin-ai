package provider

import (
	"context"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

// PreparedProvider separates local resolution, one native generation and final
// validated publication. Private provider state NEVER travels in Event or Usage.
type PreparedProvider interface {
	Provider
	Prepare(context.Context, Request) (PreparedGeneration, error)
}
type PreparedGeneration interface {
	Stream(context.Context, func(Event) error) (Usage, error)
	Publish(context.Context, tools.Batch, func(context.Context, []tools.Call) error) error
	Outcome() GenerationOutcome
	Close()
}

// GenerationOutcome is a content-free closed observation, not publication
// authority. Turn selects success only after its sole successful done flush.
type GenerationOutcome struct{ handled, replayed bool }

func NewGenerationOutcome(handled, replayed bool) GenerationOutcome {
	return GenerationOutcome{handled, replayed}
}
func (o GenerationOutcome) Handled() bool  { return o.handled }
func (o GenerationOutcome) Replayed() bool { return o.replayed }
