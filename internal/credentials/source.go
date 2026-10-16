// Package credentials defines the provider-bound boundary; production login is
// deliberately deferred to Phase 7. No probe or third-party store is imported.
package credentials

import "context"

// Identity scopes cached data. Epoch must change on reauthentication (even to
// the same account), but not ordinary access-token rotation. Never log identity.
type Identity struct {
	Provider, Account string
	Epoch             uint64
}

// Credential contains secrets and must not be logged.
type Credential struct {
	Identity    Identity
	AccessToken string
}

func (Credential) String() string   { return "[private credential]" }
func (Credential) GoString() string { return "[private credential]" }

// Source returns a currently usable credential, or an error requiring no cached
// reuse. Current must be concurrency-safe and return an atomic identity/token
// pair. Lifecycle writers publish the new source state, then synchronously
// invalidate dependent caches before completing logout, reauthentication or
// account-change operations (even reauthentication to the same account).
type Source interface {
	Current(context.Context) (Credential, error)
}
type SourceFunc func(context.Context) (Credential, error)

func (f SourceFunc) Current(ctx context.Context) (Credential, error) { return f(ctx) }
