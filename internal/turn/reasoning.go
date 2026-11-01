package turn

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

type ReasoningObserver func(ReasoningReport)

// Closed content-free snapshot. Elapsed begins before local Prepare/generation
// (after initial statuses) and ends at successful done flush, excluding logging.
type ReasoningReport struct {
	kind    uint8
	elapsed int64
}

func NewReasoningReport(o provider.GenerationOutcome, elapsed time.Duration) (ReasoningReport, bool) {
	r := ReasoningReport{elapsed: elapsed.Milliseconds()}
	if r.elapsed < 0 {
		r.elapsed = 0
	}
	if o.Replayed() {
		r.kind = 2
		if o.Handled() {
			r.kind = 3
		}
	} else if o.Handled() {
		r.kind = 1
	}
	return r, r.kind != 0
}
func (r ReasoningReport) Message() string {
	switch r.kind {
	case 1:
		return "generation_reasoning_handled"
	case 2, 3:
		return "generation_reasoning_replayed"
	default:
		return "unknown"
	}
}
func (r ReasoningReport) Category() string {
	switch r.kind {
	case 1:
		return "reasoning_handled"
	case 2:
		return "reasoning_replayed"
	case 3:
		return "reasoning_replayed_and_handled"
	default:
		return "unknown"
	}
}
func (r ReasoningReport) ElapsedMS() int64 { return r.elapsed }
func (r ReasoningReport) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Category string `json:"category"`
		Elapsed  int64  `json:"elapsed_ms"`
	}{r.Category(), r.elapsed})
}
func (r ReasoningReport) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, r.Category()) }
func (r *ReasoningReport) LogValue() slog.Value {
	if r == nil {
		return slog.StringValue("unknown")
	}
	return slog.StringValue(r.Category())
}
func observeReasoning(observer ReasoningObserver, report ReasoningReport) {
	defer func() { _ = recover() }()
	observer(report)
}
