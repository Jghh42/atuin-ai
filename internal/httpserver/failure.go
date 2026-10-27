package httpserver

import (
	"log/slog"

	"github.com/Jghh42/atuin-ai/internal/turn"
)

// NewFailureObserver uses only closed snapshot fields. Production must supply a
// fresh plain JSON logger without inherited request attributes. Logging is
// synchronous trusted local I/O: blocked stderr may hold admission AFTER the
// terminal attempt and upstream cleanup. Writer errors/panics never add wire data.
func NewFailureObserver(logger *slog.Logger) turn.FailureObserver {
	if logger == nil {
		return nil
	}
	return func(report turn.FailureReport) {
		defer func() { _ = recover() }()
		f := report.Failure()
		var status any
		if f.HTTPStatus() != 0 {
			status = f.HTTPStatus()
		}
		logger.Error("generation_failed", "phase", f.Phase().String(), "category", f.Category().String(), "upstream_http_status", status, "elapsed_ms", report.ElapsedMS())
	}
}
