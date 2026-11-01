package credentials

import (
	"context"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type dependentProbe struct {
	mu                                  sync.Mutex
	ctx                                 context.Context
	cancel                              context.CancelFunc
	nativeDone                          chan struct{}
	suspendEntered                      chan struct{}
	enteredOnce                         sync.Once
	suspends, purges, activates, closes int
	purgeError                          bool
	epochs                              []uint64
}

func (d *dependentProbe) Activate(id Identity) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.activates++
	d.epochs = append(d.epochs, id.Epoch)
}
func (d *dependentProbe) Suspend(_ SuspendReason) {
	d.mu.Lock()
	d.suspends++
	if d.cancel != nil {
		d.cancel()
	}
	done := d.nativeDone
	if d.suspendEntered != nil {
		d.enteredOnce.Do(func() { close(d.suspendEntered) })
	}
	d.mu.Unlock()
	if done != nil {
		<-done
	}
}
func (d *dependentProbe) Purge(intent DurableIntent) error {
	if !intent.Valid() {
		panic("invalid durable authority")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.purges++
	if d.purgeError {
		return ErrStore
	}
	return nil
}
func (d *dependentProbe) Close() error { d.mu.Lock(); defer d.mu.Unlock(); d.closes++; return nil }
func TestFailedBeginAlwaysSuspendsAndJoinsPrivateStreams(t *testing.T) {
	for _, operation := range []string{"login", "logout", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			s, _ := testStore(t)
			expiry := testNow.Add(time.Hour)
			if operation == "refresh" {
				expiry = testNow.Add(time.Second)
			}
			seed(t, s, activeRecord(expiry))
			var external atomic.Int32
			d := &dependentProbe{suspendEntered: make(chan struct{})}
			m, e := NewManager(s, ManagerOptions{Dependent: d, OAuth: OAuthClient{Now: func() time.Time { return testNow }, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { external.Add(1); return nil, ErrOAuth })}})
			if e != nil {
				t.Fatal(e)
			}
			defer m.Close()
			d.mu.Lock()
			scope := d.ctx
			nativeDone := make(chan struct{})
			d.nativeDone = nativeDone
			d.mu.Unlock()
			cleanup := make(chan struct{})
			privateStopped := make(chan struct{})
			go func() { <-scope.Done(); close(privateStopped); <-cleanup; close(nativeDone) }()
			s.fault = func(stage string) error {
				if stage == "intent.json.write" {
					return ErrStore
				}
				return nil
			}
			result := make(chan error, 1)
			go func() {
				switch operation {
				case "login":
					result <- m.Login(context.Background(), func(DevicePrompt) error { return nil })
				case "logout":
					result <- m.Logout(context.Background())
				case "refresh":
					_, e := m.Current(context.Background())
					result <- e
				}
			}()
			<-d.suspendEntered
			<-privateStopped
			select {
			case <-result:
				t.Fatal("lifecycle returned before native callback cleanup")
			default:
			}
			// Completed transaction awaiting the held operation gate must exit on the
			// revoked original scope, not await its own Close from Suspend.
			called := false
			pending := make(chan error, 1)
			go func() {
				pending <- m.WithIdentityPublication(scope, Identity{Provider: "codex", Account: "account-test", Epoch: 1}, func(PublicationLease) error { called = true; return nil })
			}()
			select {
			case e := <-pending:
				if e == nil || called {
					t.Fatal("revoked pending publication authorized")
				}
			case <-time.After(time.Second):
				t.Fatal("pending publication deadlocked on lifecycle gate")
			}
			close(cleanup)
			select {
			case e := <-result:
				if e != ErrStore {
					t.Fatal("failed Begin lost fixed error", e)
				}
			case <-time.After(time.Second):
				t.Fatal("lifecycle did not join")
			}
			d.mu.Lock()
			purges := d.purges
			d.mu.Unlock()
			if purges != 0 || external.Load() != 0 {
				t.Fatal("failed Begin purged or contacted OAuth")
			}
			if m.Status().LoggedIn {
				t.Fatal("failed intent left usable in-memory identity")
			}
			loaded, e := s.Load()
			if e != nil || loaded.State != "active" {
				t.Fatal("failed Begin incorrectly claimed durable logout")
			}
		})
	}
}
func TestLogoutPurgeFailureStillCommitsCredentialTombstone(t *testing.T) {
	s, _ := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Hour)))
	d := &dependentProbe{purgeError: true}
	m, e := NewManager(s, ManagerOptions{Dependent: d, OAuth: OAuthClient{Now: func() time.Time { return testNow }}})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if e = m.Logout(context.Background()); e != ErrStore {
		t.Fatal("purge error hidden")
	}
	r, e := s.Load()
	if e != nil || r.State != "logged_out" {
		t.Fatal("purge failure skipped credential Commit")
	}
}
func TestRefreshPreservesEpochAndDoesNotPurge(t *testing.T) {
	s, _ := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Second)))
	d := &dependentProbe{}
	m, e := NewManager(s, ManagerOptions{Dependent: d, OAuth: OAuthClient{Now: func() time.Time { return testNow }, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(200, tokenJSON("account-test", "replacement", testNow.Add(time.Hour))), nil
	})}})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if _, e = m.Current(context.Background()); e != nil {
		t.Fatal(e)
	}
	if d.suspends != 1 || d.purges != 0 || d.activates != 2 || d.epochs[0] != d.epochs[1] {
		t.Fatal("refresh destroyed durable scope")
	}
}
func TestCounterFailuresSuspendWithoutPurge(t *testing.T) {
	for _, op := range []string{"login", "logout", "refresh"} {
		t.Run(op, func(t *testing.T) {
			s, _ := testStore(t)
			r := activeRecord(testNow.Add(time.Second))
			r.Generation = math.MaxUint64
			seed(t, s, r)
			d := &dependentProbe{}
			m, e := NewManager(s, ManagerOptions{Dependent: d, OAuth: OAuthClient{Now: func() time.Time { return testNow }}})
			if e != nil {
				t.Fatal(e)
			}
			defer m.Close()
			switch op {
			case "login":
				e = m.Login(context.Background(), func(DevicePrompt) error { return nil })
			case "logout":
				e = m.Logout(context.Background())
			case "refresh":
				_, e = m.Current(context.Background())
			}
			if e != ErrStore || m.Status().LoggedIn || d.suspends != 1 || d.purges != 0 {
				t.Fatal("counter failure retained authority")
			}
		})
	}
}
func TestPublicationGateIdentityLifetimeAndDiagnostics(t *testing.T) {
	s, _ := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Hour)))
	m := manager(t, s, nil, nil)
	id := Identity{Provider: "codex", Account: "account-test", Epoch: 1}
	var saved PublicationLease
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e := m.WithIdentityPublication(ctx, id, func(l PublicationLease) error {
		saved = l
		if l.Check() != nil {
			t.Fatal("fresh lease unavailable")
		}
		if result := m.WithDiagnosticAccess(ctx, func(context.Context, Credential) { t.Fatal("diagnostic reentered held gate") }); result != DiagnosticUnavailable {
			t.Fatal("diagnostic no longer fail-fast")
		}
		cancel()
		if l.Context().Err() != context.Canceled || l.Check() == nil {
			t.Fatal("original cancellation waited on AfterFunc")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if saved.Check() == nil || saved.Context().Err() == nil {
		t.Fatal("escaped lease retained authority")
	}
	wrong := id
	wrong.Epoch++
	if e := m.WithIdentityPublication(context.Background(), wrong, func(PublicationLease) error { t.Fatal("wrong identity published"); return nil }); e == nil {
		t.Fatal("wrong identity admitted")
	}
	// A canceled waiter must not need a helper goroutine or gate release.
	m.op.Lock()
	waitCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.WithIdentityPublication(waitCtx, id, func(PublicationLease) error { return nil }) }()
	stop()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("canceled waiter acquired gate")
		}
	case <-time.After(time.Second):
		t.Fatal("gate wait ignored cancellation")
	}
	m.op.Unlock()
}
func TestCloseCancelsPublicationSourceBeforeJoiningGate(t *testing.T) {
	s, _ := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Hour)))
	m := manager(t, s, nil, nil)
	id := Identity{Provider: "codex", Account: "account-test", Epoch: 1}
	entered, release := make(chan struct{}), make(chan struct{})
	published := make(chan error, 1)
	go func() {
		published <- m.WithIdentityPublication(context.Background(), id, func(l PublicationLease) error {
			close(entered)
			<-m.life.Done()
			if l.Context().Err() == nil || l.Check() == nil {
				t.Error("Close direct lifetime source ignored")
			}
			<-release
			return nil
		})
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- m.Close() }()
	<-m.life.Done()
	select {
	case <-closed:
		t.Fatal("Close did not join publication")
	default:
	}
	close(release)
	if e := <-published; e != nil {
		t.Fatal("callback terminal result overridden")
	}
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
}
