package turn

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

// FailureObserver runs synchronously after terminal flush attempt, callback
// closure/unlock and turn cancellation/upstream cleanup. A blocked trusted
// writer can retain the handler/admission slot; there is no background worker.
type FailureObserver func(FailureReport)

type FailureReport struct {
	failure provider.Failure
	elapsed int64
}

func NewFailureReport(f provider.Failure, elapsed time.Duration) FailureReport {
	ms := elapsed.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return FailureReport{f, ms}
}
func (r FailureReport) Failure() provider.Failure { return r.failure }
func (r FailureReport) ElapsedMS() int64          { return r.elapsed }
func (r FailureReport) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Failure provider.Failure `json:"failure"`
		Elapsed int64            `json:"elapsed_ms"`
	}{r.failure, r.elapsed})
}
func (r FailureReport) Format(s fmt.State, _ rune) { b, _ := r.MarshalJSON(); _, _ = s.Write(b) }
func (r FailureReport) LogValue() slog.Value {
	return slog.GroupValue(slog.Any("failure", r.failure), slog.Int64("elapsed_ms", r.elapsed))
}
func observe(observer FailureObserver, report FailureReport) {
	defer func() { _ = recover() }()
	observer(report)
}
