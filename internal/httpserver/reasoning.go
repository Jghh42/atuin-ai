package httpserver

import (
	"github.com/Jghh42/atuin-ai/internal/turn"
	"log/slog"
)

// Exactly the existing seven plain-JSON keys. Logger is trusted synchronous I/O
// AFTER terminal/transaction/native cleanup, never a queue or streaming sink.
func NewReasoningObserver(logger *slog.Logger) turn.ReasoningObserver {
	if logger == nil {
		return nil
	}
	return func(report turn.ReasoningReport) {
		defer func() { _ = recover() }()
		if report.Message() == "unknown" {
			return
		}
		logger.Info(report.Message(), "phase", "stream", "category", report.Category(), "upstream_http_status", nil, "elapsed_ms", report.ElapsedMS())
	}
}
