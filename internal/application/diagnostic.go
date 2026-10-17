package application

import (
	"context"

	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

// Diagnose is an explicit remote check, independent of bundled availability. It
// uses the manager's no-refresh borrowing boundary; no Current, OAuth, inference,
// cache mutation, or lifecycle transaction is performed.
func (r *Runtime) Diagnose(ctx context.Context) codex.DiagnosticReport {
	configured := r.defaultModel != ""
	report := codex.DiagnosticReport{Stage: codex.StageCredentials, Result: codex.ResultCredentialUnavailable, DefaultOverrideConfigured: &configured}
	access := r.Manager.WithDiagnosticAccess(ctx, func(bound context.Context, c credentials.Credential) {
		report = r.discovery.Diagnose(bound, c, r.defaultModel)
	})
	if access == credentials.DiagnosticReady {
		return report
	}
	// Do not publish measurements from a canceled/invalidated borrowed identity.
	report = codex.DiagnosticReport{Stage: codex.StageCredentials, Result: codex.ResultCredentialUnavailable, DefaultOverrideConfigured: &configured}
	switch access {
	case credentials.DiagnosticRefreshRequired:
		report.Result = codex.ResultRefreshRequired
	case credentials.DiagnosticLifecycleInvalidated:
		report.Result = codex.ResultLifecycleInvalidated
	case credentials.DiagnosticCanceled:
		report.Result = codex.ResultCanceled
		if ctx.Err() == context.DeadlineExceeded {
			report.Result = codex.ResultTimeout
		}
	}
	return report
}
