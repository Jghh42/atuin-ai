package credentials

import "context"

// DiagnosticAccessResult describes local access only. It contains no identity,
// token, path, or upstream error. Diagnostic access NEVER rotates or saves state.
type DiagnosticAccessResult uint8

const (
	DiagnosticUnavailable DiagnosticAccessResult = iota
	DiagnosticReady
	DiagnosticRefreshRequired
	DiagnosticLifecycleInvalidated
	DiagnosticCanceled
)

// WithDiagnosticAccess lends one currently usable atomic identity/token pair to
// a bounded diagnostic callback, without calling Current or refreshing. The
// lifecycle operation lock prevents login/logout/rotation from changing the
// borrowed session. Contention fails immediately rather than waiting for OAuth.
// Close can still win: its lifetime cancellation stops the callback and the
// final state check rejects the result before releasing the operation lock.
// Callbacks must not retain credentials, log them, or invoke manager operations.
func (m *Manager) WithDiagnosticAccess(ctx context.Context, inspect func(context.Context, Credential)) DiagnosticAccessResult {
	if ctx.Err() != nil {
		return DiagnosticCanceled
	}
	if inspect == nil || !m.op.TryLock() {
		return DiagnosticUnavailable
	}
	defer m.op.Unlock()
	m.mu.Lock()
	if m.closing || m.closed {
		m.mu.Unlock()
		return DiagnosticLifecycleInvalidated
	}
	r := m.record
	if r.State != "active" || m.flight != nil {
		m.mu.Unlock()
		return DiagnosticUnavailable
	}
	if !usable(r, m.oauth.now()) {
		m.mu.Unlock()
		return DiagnosticRefreshRequired
	}
	c := credential(r)
	m.mu.Unlock()

	bound, stop := m.operationContext(ctx)
	defer stop()
	// Close may have won between taking the record and binding cancellation.
	if bound.Err() == nil {
		inspect(bound, c)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || m.closed || m.life.Err() != nil || m.record.Generation != r.Generation || m.record.AuthEpoch != r.AuthEpoch || m.record.State != "active" {
		return DiagnosticLifecycleInvalidated
	}
	if ctx.Err() != nil {
		return DiagnosticCanceled
	}
	return DiagnosticReady
}
