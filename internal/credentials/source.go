// Package credentials owns this application's native Codex credential lifecycle.
// No probe or third-party store is imported.
package credentials

import "context"

// Identity scopes cached data. Epoch must change on reauthentication (even to
// the same account), but not ordinary access-token rotation. Never log identity.
type Identity struct {
	Provider, Account string
	Epoch             uint64
}

func (Identity) String() string   { return "[private credential identity]" }
func (Identity) GoString() string { return "[private credential identity]" }

type identityKey struct{}

// BindIdentity pins discovery's identity through the later inference lookup.
// A lifecycle change must reject the turn, never rebind it to another account.
func BindIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}
func MatchesIdentity(ctx context.Context, id Identity) bool {
	expected, ok := ctx.Value(identityKey{}).(Identity)
	return !ok || expected == id
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
