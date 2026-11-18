package credentials

import (
	"context"
	"math"
	"sync"
	"time"
)

const AccessSkew = time.Minute
const persistenceTimeout = 5 * time.Second

type ManagerOptions struct {
	OAuth OAuthClient
	// Invalidate is fixed at construction, synchronously invoked outside state
	// locks AFTER publishing lifecycle state. Composition binds one cache once.
	Invalidate func()
	// Dependent is optional; nil preserves the legacy runtime composition.
	Dependent LifecycleDependent
}
type refreshFlight struct {
	done chan struct{}
	err  error
}

// Manager owns one store and one rotating session. Caller cancellation never
// cancels other Current waiters, or a rotation/persistence already in progress.
type Manager struct {
	mu           sync.Mutex
	op           operationGate
	record       Record
	store        Storage
	oauth        OAuthClient
	invalidate   func()
	dependent    LifecycleDependent
	flight       *refreshFlight
	closing      bool
	closed       bool
	shutdownDone chan struct{}
	shutdownErr  error
	life         context.Context
	stop         context.CancelFunc
	wg           sync.WaitGroup
}

func (*Manager) String() string   { return "[private credential manager]" }
func (*Manager) GoString() string { return "[private credential manager]" }
func NewManager(store Storage, opts ManagerOptions) (*Manager, error) {
	if store == nil {
		return nil, ErrStore
	}
	r, e := store.Load()
	if e != nil || r != emptyRecord() && r.validate() != nil {
		return nil, ErrStore
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{record: r, store: store, oauth: opts.OAuth, invalidate: opts.Invalidate, dependent: opts.Dependent, life: ctx, stop: cancel, shutdownDone: make(chan struct{})}
	if m.dependent != nil {
		if r.State == "active" {
			m.activate(r)
		} else {
			m.suspend(SuspendStartup)
			if r.Generation > 0 && m.purge() != nil {
				cancel()
				_ = m.dependent.Close()
				return nil, ErrStore
			}
		}
	}
	return m, nil
}
func (m *Manager) invalidated() {
	if m.invalidate != nil {
		m.invalidate()
	}
}
func (m *Manager) publish(r Record) { m.mu.Lock(); m.record = r; m.mu.Unlock(); m.invalidated() }
func usable(r Record, now time.Time) bool {
	return r.State == "active" && r.ExpiresAt.After(now.Add(AccessSkew)) && !now.Before(r.IssuedAt)
}
func credential(r Record) Credential {
	return Credential{Identity: Identity{Provider: r.Provider, Account: r.Account, Epoch: r.AuthEpoch}, AccessToken: r.Access}
}
func (m *Manager) Current(ctx context.Context) (Credential, error) {
	if ctx.Err() != nil {
		return Credential{}, ctx.Err()
	}
	m.mu.Lock()
	if m.closing || m.closed {
		m.mu.Unlock()
		return Credential{}, ErrUnavailable
	}
	f := m.flight
	if f == nil {
		if m.record.State != "active" {
			m.mu.Unlock()
			return Credential{}, ErrReauth
		}
		if usable(m.record, m.oauth.now()) {
			c := credential(m.record)
			m.mu.Unlock()
			return c, nil
		}
		f = &refreshFlight{done: make(chan struct{})}
		m.flight = f
		old := m.record
		m.wg.Add(1)
		go m.rotate(old, f)
	}
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		return Credential{}, ctx.Err()
	case <-f.done:
		if ctx.Err() != nil {
			return Credential{}, ctx.Err()
		}
		if f.err != nil {
			return Credential{}, f.err
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closing || m.closed || !usable(m.record, m.oauth.now()) {
			return Credential{}, ErrReauth
		}
		return credential(m.record), nil
	}
}
func next(old Record, login bool, now time.Time) (Record, error) {
	if old.Generation == math.MaxUint64 || login && old.AuthEpoch == math.MaxUint64 {
		return Record{}, ErrStore
	}
	r := tombstone(old, "unusable", now)
	r.Generation++
	if login {
		r.AuthEpoch++
	}
	return r, nil
}
func (m *Manager) rotate(old Record, f *refreshFlight) {
	defer m.wg.Done()
	m.op.Lock()
	defer m.op.Unlock()
	var err error
	defer func() {
		if err != nil {
			m.invalidated()
		}
		m.mu.Lock()
		f.err = err
		if m.flight == f {
			m.flight = nil
		}
		close(f.done)
		m.mu.Unlock()
	}()
	m.mu.Lock()
	same := !m.closing && !m.closed && m.record.Generation == old.Generation && m.record.State == "active"
	m.mu.Unlock()
	if !same {
		err = ErrReauth
		return
	}
	intent, e := next(old, false, m.oauth.now())
	if e != nil {
		m.publishUnusable(tombstone(old, "unusable", m.oauth.now()), SuspendUnusable)
		err = ErrStore
		return
	}
	m.publishUnusable(intent, SuspendRefresh)
	ctx, stop := context.WithTimeout(m.life, 30*time.Second)
	defer stop()
	if m.store.Begin(ctx, intent) != nil {
		err = ErrStore
		return
	}
	// Intent is durable before the ONLY external refresh attempt. Any ambiguous
	// transport/malformed/invalid_grant/save failure leaves it unusable on restart.
	replacement, e := m.oauth.refresh(ctx, old)
	if e == errNotSent {
		// The old refresh token never left this machine (offline, DNS, refused
		// connection), so durably restore it and let a later request retry.
		replacement, e, err = old, nil, ErrUnavailable
	}
	if e != nil {
		_ = m.purge() // successful Begin; terminal unusable state
		err = ErrReauth
		return
	}
	replacement.Generation = intent.Generation
	replacement.AuthEpoch = intent.AuthEpoch
	replacement.SavedAt = m.oauth.now()
	if m.persist(replacement) != nil {
		_ = m.purge()
		err = ErrStore
		return
	}
	m.mu.Lock()
	m.record = replacement
	m.mu.Unlock() // rotation preserves epoch/catalog
	m.activate(replacement)
}
func (m *Manager) persist(r Record) error {
	// Detach from callers and shutdown once tokens have arrived. The store checks
	// this finite budget between filesystem operations; kernel fsync itself is
	// synchronous and subject to OS/filesystem availability.
	ctx, cancel := context.WithTimeout(context.Background(), persistenceTimeout)
	defer cancel()
	return m.store.Commit(ctx, r)
}
func (m *Manager) operationContext(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.life, cancel)
	return ctx, func() { stop(); cancel() }
}

// Login explicitly replaces this application's session, including the same
// account. Catalog membership/diagnosis is separate from saving login.
func (m *Manager) Login(ctx context.Context, display func(DevicePrompt) error) error {
	ctx, stop := m.operationContext(ctx)
	defer stop()
	if m.op.LockContext(ctx) != nil {
		return ErrUnavailable
	}
	defer m.op.Unlock()
	m.mu.Lock()
	old := m.record
	unavailable := m.closing || m.closed
	m.mu.Unlock()
	if unavailable || ctx.Err() != nil || display == nil {
		return ErrUnavailable
	}
	intent, e := next(old, true, m.oauth.now())
	if e != nil {
		if m.dependent != nil {
			m.publishUnusable(tombstone(old, "unusable", m.oauth.now()), SuspendUnusable)
		}
		return e
	}
	m.publishUnusable(intent, SuspendReauth)
	if m.store.Begin(ctx, intent) != nil {
		return ErrStore
	}
	if m.purge() != nil {
		return ErrStore
	}
	r, e := m.oauth.login(ctx, display)
	if e != nil {
		return e
	}
	r.Generation = intent.Generation
	r.AuthEpoch = intent.AuthEpoch
	r.SavedAt = m.oauth.now()
	if m.persist(r) != nil {
		return ErrStore
	}
	m.publish(r)
	m.activate(r)
	return nil
}

// Logout is durable LOCAL secret deletion. No remote revocation endpoint is
// verified; a secret-free tombstone retains monotonic counters.
func (m *Manager) Logout(ctx context.Context) error {
	bound, stop := m.operationContext(ctx)
	defer stop()
	if m.op.LockContext(bound) != nil {
		return ErrUnavailable
	}
	defer m.op.Unlock()
	m.mu.Lock()
	old := m.record
	unavailable := m.closing || m.closed
	m.mu.Unlock()
	if unavailable || ctx.Err() != nil {
		return ErrUnavailable
	}
	intent, e := next(old, false, m.oauth.now())
	if e != nil {
		if m.dependent != nil {
			m.publishUnusable(tombstone(old, "unusable", m.oauth.now()), SuspendUnusable)
		}
		return e
	}
	m.publishUnusable(intent, SuspendLogout)
	if m.store.Begin(bound, intent) != nil {
		return ErrStore
	}
	purgeErr := m.purge()
	r := intent
	r.State = "logged_out"
	if m.persist(r) != nil {
		return ErrStore
	}
	m.publish(r)
	if purgeErr != nil {
		return ErrStore
	}
	return nil
}

// Status contains no account IDs, tokens, or filesystem paths, and never refreshes.
type Status struct {
	LoggedIn                 bool
	ReauthenticationRequired bool
	ExpiresAt                time.Time
	Generation, AuthEpoch    uint64
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.record
	return Status{LoggedIn: !m.closing && !m.closed && r.State == "active", ReauthenticationRequired: r.State == "unusable", ExpiresAt: r.ExpiresAt, Generation: r.Generation, AuthEpoch: r.AuthEpoch}
}

// Close is a shared shutdown barrier, not just an availability flag. EVERY
// caller joins cancellation, detached persistence, workers and lifetime-store
// release, then observes the same immutable store-close result.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closing || m.closed {
		done := m.shutdownDone
		m.mu.Unlock()
		<-done
		// shutdownErr is immutable after the channel-close publication barrier.
		return m.shutdownErr
	}
	m.closing = true
	done := m.shutdownDone
	m.mu.Unlock()
	m.stop()
	m.suspend(SuspendShutdown)
	m.invalidated()
	m.wg.Wait()
	m.op.Lock()
	var err error
	if m.dependent != nil && m.dependent.Close() != nil {
		err = ErrStore
	}
	if storeErr := m.store.Close(); storeErr != nil && err == nil {
		err = storeErr
	}
	m.op.Unlock()
	m.mu.Lock()
	m.shutdownErr = err
	m.closed = true
	m.closing = false
	close(done)
	m.mu.Unlock()
	return err
}
