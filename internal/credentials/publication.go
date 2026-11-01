package credentials

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"
)

// SuspendReason is closed; no credential/error text crosses this interface.
type SuspendReason uint8

const (
	SuspendStartup SuspendReason = iota
	SuspendReauth
	SuspendLogout
	SuspendRefresh
	SuspendUnusable
	SuspendShutdown
)

// DurableIntent is minted only after successful Begin (or a loaded durable
// tombstone). It conveys authority to purge, not credentials or filesystem data.
type DurableIntent struct{ valid bool }

func (d DurableIntent) Valid() bool { return d.valid }

// LifecycleDependent callbacks run outside manager.mu. They must not call back
// into Manager. Suspend always joins started streams, never pending publication.
// Close preserves committed data and completes before the credential flock ends.
type LifecycleDependent interface {
	Suspend(SuspendReason)
	Purge(DurableIntent) error
	Activate(Identity)
	Close() error
}

// PublicationLease is ephemeral authority under the operation gate. Check reads
// the ORIGINAL caller and manager lifetime sources synchronously, not just an
// AfterFunc-linked cancellation. It must not escape the callback.
type PublicationLease struct{ state *publicationState }

// Only unexported payload pointers are traversable on opaque presentations.
// Invalid fmt verbs bypass all formatter methods in Go1.27; contexts may be
// custom value structs carrying secrets and must never be embedded/exported.
type publicationState struct {
	publicationRedactor
	payload *publicationStatePayload
}
type publicationStatePayload struct {
	original, life, linked context.Context
	revoked                atomic.Bool
}
type publicationContext struct {
	publicationRedactor
	payload *publicationContextPayload
}
type publicationContextPayload struct {
	linked context.Context
	state  *publicationState
}
type publicationRedactor struct{}

func (publicationRedactor) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "[private publication]")
}
func (publicationRedactor) MarshalJSON() ([]byte, error) {
	return []byte(`"[private publication]"`), nil
}
func (*publicationState) LogValue() slog.Value   { return slog.StringValue("[private publication]") }
func (*publicationContext) LogValue() slog.Value { return slog.StringValue("[private publication]") }
func newPublicationContext(state *publicationState) publicationContext {
	return publicationContext{payload: &publicationContextPayload{state: state, linked: state.payload.linked}}
}
func (c publicationContext) Deadline() (time.Time, bool) {
	if c.payload == nil {
		return time.Time{}, false
	}
	return c.payload.linked.Deadline()
}
func (c publicationContext) Done() <-chan struct{} {
	if c.payload == nil {
		return canceledPublicationContext.Done()
	}
	return c.payload.linked.Done()
}
func (c publicationContext) Value(key any) any {
	if c.payload == nil {
		return nil
	}
	return c.payload.linked.Value(key)
}

var canceledPublicationContext = func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }()

func (c publicationContext) Err() error {
	if c.payload == nil || c.payload.state == nil {
		return context.Canceled
	}
	state := c.payload.state.payload
	if e := state.original.Err(); e != nil {
		return e
	}
	if e := state.life.Err(); e != nil {
		return e
	}
	if state.revoked.Load() {
		return context.Canceled
	}
	return c.payload.linked.Err()
}
func (l PublicationLease) Check() error {
	if l.state == nil {
		return ErrUnavailable
	}
	if newPublicationContext(l.state).Err() != nil {
		return ErrUnavailable
	}
	return nil
}
func (l PublicationLease) Context() context.Context {
	if l.state == nil {
		c, cancel := context.WithCancel(context.Background())
		cancel()
		return c
	}
	return newPublicationContext(l.state)
}
func (PublicationLease) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "[private publication lease]")
}
func (PublicationLease) MarshalJSON() ([]byte, error) {
	return []byte(`"[private publication lease]"`), nil
}
func (*PublicationLease) LogValue() slog.Value {
	return slog.StringValue("[private publication lease]")
}

// WithIdentityPublication serializes final publication against all lifecycle
// writers. No network generation is permitted here. Callback must not invoke
// Current, Login, Logout or Close. No manager state lock is held across callback.
func (m *Manager) WithIdentityPublication(ctx context.Context, expected Identity, publish func(PublicationLease) error) error {
	linked, stop := m.operationContext(ctx)
	defer stop()
	if publish == nil || m.op.LockContext(linked) != nil {
		return ErrUnavailable
	}
	defer m.op.Unlock()
	m.mu.Lock()
	valid := !m.closing && !m.closed && m.record.State == "active" && credential(m.record).Identity == expected
	m.mu.Unlock()
	if !valid || ctx.Err() != nil || m.life.Err() != nil {
		return ErrUnavailable
	}
	state := &publicationState{payload: &publicationStatePayload{original: ctx, life: m.life, linked: linked}}
	defer state.payload.revoked.Store(true)
	lease := PublicationLease{state}
	if lease.Check() != nil {
		return ErrUnavailable
	}
	return publish(lease)
}
func (m *Manager) suspend(reason SuspendReason) {
	if m.dependent != nil {
		m.dependent.Suspend(reason)
	}
}
func (m *Manager) publishUnusable(r Record, reason SuspendReason) {
	if reason == SuspendRefresh {
		m.mu.Lock()
		m.record = r
		m.mu.Unlock()
	} else {
		m.mu.Lock()
		m.record = r
		m.mu.Unlock()
	}
	m.suspend(reason)
	if reason != SuspendRefresh {
		m.invalidated()
	}
}
func (m *Manager) activate(r Record) {
	if m.dependent != nil && m.life.Err() == nil {
		m.dependent.Activate(credential(r).Identity)
	}
}
func (m *Manager) purge() error {
	if m.dependent != nil && m.dependent.Purge(DurableIntent{true}) != nil {
		return ErrStore
	}
	return nil
}
