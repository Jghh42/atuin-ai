package credentials

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

type diagnosticStorage struct {
	Storage
	begins, commits atomic.Int32
	callbackActive  *atomic.Bool
}

func (s *diagnosticStorage) Begin(ctx context.Context, r Record) error {
	s.begins.Add(1)
	if s.callbackActive != nil && s.callbackActive.Load() {
		return ErrStore
	}
	return s.Storage.Begin(ctx, r)
}
func (s *diagnosticStorage) Commit(ctx context.Context, r Record) error {
	s.commits.Add(1)
	return s.Storage.Commit(ctx, r)
}

func TestDiagnosticAccessNeverRefreshesOrWrites(t *testing.T) {
	for _, tc := range []struct {
		name    string
		record  Record
		pending bool
		want    DiagnosticAccessResult
	}{
		{"active", activeRecord(testNow.Add(time.Hour)), false, DiagnosticReady},
		{"skew boundary", activeRecord(testNow.Add(AccessSkew)), false, DiagnosticRefreshRequired},
		{"near expiry", activeRecord(testNow.Add(AccessSkew - time.Second)), false, DiagnosticRefreshRequired},
		{"expired", activeRecord(testNow.Add(-time.Second)), false, DiagnosticRefreshRequired},
		{"clock before issue", activeRecord(testNow.Add(time.Hour)), false, DiagnosticRefreshRequired},
		{"logged out", tombstone(activeRecord(testNow.Add(time.Hour)), "logged_out", testNow), false, DiagnosticUnavailable},
		{"pending intent", activeRecord(testNow.Add(time.Hour)), true, DiagnosticUnavailable},
		{"unusable", tombstone(activeRecord(testNow.Add(time.Hour)), "unusable", testNow), false, DiagnosticUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := testStore(t)
			if tc.record.State == "unusable" {
				if s.Begin(context.Background(), tc.record) != nil {
					t.Fatal("unusable intent fixture failed")
				}
			} else {
				seed(t, s, tc.record)
			}
			if tc.pending {
				intent := tombstone(tc.record, "unusable", testNow)
				intent.Generation++
				if s.Begin(context.Background(), intent) != nil {
					t.Fatal("pending fixture failed")
				}
			}
			before, err := s.Load()
			if err != nil {
				t.Fatal(err)
			}
			writes := &diagnosticStorage{Storage: s}
			var requests atomic.Int32
			m := manager(t, writes, roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return nil, ErrOAuth
			}), nil)
			if tc.name == "clock before issue" {
				m.oauth.Now = func() time.Time { return testNow.Add(-2 * time.Minute) }
			}
			status := m.Status()
			callbacks := 0
			got := m.WithDiagnosticAccess(context.Background(), func(ctx context.Context, c Credential) {
				callbacks++
				if ctx.Err() != nil || c.AccessToken != before.Access || c.Identity.Account != before.Account || c.Identity.Epoch != before.AuthEpoch {
					t.Error("diagnostic credential pair not atomic")
				}
			})
			after, err := s.Load()
			if err != nil || !reflect.DeepEqual(before, after) || m.Status() != status || writes.begins.Load() != 0 || writes.commits.Load() != 0 || requests.Load() != 0 {
				t.Fatal("diagnostic mutated state or refreshed")
			}
			wantCallbacks := 0
			if tc.want == DiagnosticReady {
				wantCallbacks = 1
			}
			if got != tc.want || callbacks != wantCallbacks {
				t.Fatal("wrong no-refresh access outcome")
			}
		})
	}
}

func TestDiagnosticAccessLifecycleAndCancellation(t *testing.T) {
	for _, action := range []string{"close", "cancel", "logout", "contention", "closed", "pre-canceled", "nil callback", "refresh flight"} {
		t.Run(action, func(t *testing.T) {
			s, _ := testStore(t)
			seed(t, s, activeRecord(testNow.Add(time.Hour)))
			var active atomic.Bool
			writes := &diagnosticStorage{Storage: s, callbackActive: &active}
			m := manager(t, writes, roundTripFunc(func(*http.Request) (*http.Response, error) { t.Error("unexpected OAuth"); return nil, ErrOAuth }), nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if action == "contention" {
				m.op.Lock()
				got := m.WithDiagnosticAccess(ctx, func(context.Context, Credential) { t.Error("callback during lifecycle writer") })
				m.op.Unlock()
				if got != DiagnosticUnavailable {
					t.Fatal("contention not fail-fast")
				}
				return
			}
			if action == "closed" {
				m.Close()
				if m.WithDiagnosticAccess(ctx, func(context.Context, Credential) { t.Error("callback after close") }) != DiagnosticLifecycleInvalidated {
					t.Fatal("closed access allowed")
				}
				return
			}
			if action == "pre-canceled" {
				cancel()
				if m.WithDiagnosticAccess(ctx, func(context.Context, Credential) { t.Error("canceled callback") }) != DiagnosticCanceled {
					t.Fatal("canceled access allowed")
				}
				return
			}
			if action == "nil callback" {
				if m.WithDiagnosticAccess(ctx, nil) != DiagnosticUnavailable {
					t.Fatal("nil callback allowed")
				}
				return
			}
			if action == "refresh flight" {
				m.mu.Lock()
				m.flight = &refreshFlight{}
				m.mu.Unlock()
				if m.WithDiagnosticAccess(ctx, func(context.Context, Credential) { t.Error("callback during refresh") }) != DiagnosticUnavailable {
					t.Fatal("refresh not refused")
				}
				m.mu.Lock()
				m.flight = nil
				m.mu.Unlock()
				return
			}
			entered, release := make(chan struct{}), make(chan struct{})
			done := make(chan DiagnosticAccessResult, 1)
			go func() {
				done <- m.WithDiagnosticAccess(ctx, func(bound context.Context, _ Credential) {
					active.Store(true)
					defer active.Store(false)
					close(entered)
					if action == "logout" {
						<-release
					} else {
						<-bound.Done()
					}
				})
			}()
			<-entered
			writer := make(chan error, 1)
			want := DiagnosticReady
			switch action {
			case "close":
				want = DiagnosticLifecycleInvalidated
				go func() { writer <- m.Close() }()
			case "cancel":
				want = DiagnosticCanceled
				cancel()
			case "logout":
				if m.op.TryLock() {
					m.op.Unlock()
					t.Fatal("diagnostic failed to pin lifecycle")
				}
				go func() { writer <- m.Logout(context.Background()) }()
				close(release)
			}
			select {
			case got := <-done:
				if got != want {
					t.Fatal("stale/canceled diagnostic result published")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("diagnostic lifecycle deadlock")
			}
			if action != "cancel" {
				select {
				case err := <-writer:
					if err != nil {
						t.Fatal("lifecycle failed")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("lifecycle writer deadlock")
				}
			}
			if action != "logout" && (writes.begins.Load() != 0 || writes.commits.Load() != 0) {
				t.Fatal("diagnostic wrote credentials")
			}
		})
	}
}

func TestDiagnosticStoreOnlyOpensExistingStateWithoutCleanup(t *testing.T) {
	s, path := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Hour)))
	s.Close()
	orphan := filepath.Join(path, ".record-diagnostic-private")
	if os.WriteFile(orphan, []byte("synthetic orphan must remain"), 0600) != nil {
		t.Fatal("orphan fixture failed")
	}
	before, err := os.ReadFile(filepath.Join(path, "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := OpenDiagnosticStore(path)
	if err != nil {
		t.Fatal("existing diagnostic store failed")
	}
	if _, err := OpenDiagnosticStore(path); err != ErrStore {
		t.Fatal("diagnostic bypassed lifetime lock")
	}
	loaded, err := d.Load()
	if err != nil {
		t.Fatal("diagnostic load failed")
	}
	intent := tombstone(loaded, "unusable", testNow)
	intent.Generation++
	if d.Begin(context.Background(), intent) != ErrStore || d.Commit(context.Background(), loaded) != ErrStore {
		t.Fatal("diagnostic store allowed writes")
	}
	d.Close()
	after, err := os.ReadFile(filepath.Join(path, "record.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("diagnostic changed record")
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatal("diagnostic cleaned orphan")
	}
	missing := filepath.Join(filepath.Dir(path), "never-create", "credentials")
	if _, err := OpenDiagnosticStore(missing); err != ErrStore {
		t.Fatal("missing store accepted")
	}
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Fatal("diagnostic created ancestry")
	}
	unsafeOrphan := filepath.Join(path, ".record-unsafe-symlink")
	if os.Symlink(orphan, unsafeOrphan) != nil {
		t.Fatal("unsafe orphan fixture failed")
	}
	if _, err := OpenDiagnosticStore(path); err != ErrStore {
		t.Fatal("diagnostic weakened orphan metadata validation")
	}
	if os.Remove(unsafeOrphan) != nil {
		t.Fatal("unsafe fixture removal failed")
	}
	if os.Remove(filepath.Join(path, "lock")) != nil {
		t.Fatal("fixture unlock removal failed")
	}
	if _, err := OpenDiagnosticStore(path); err != ErrStore {
		t.Fatal("diagnostic recreated missing lock")
	}
	if _, err := os.Stat(filepath.Join(path, "lock")); !os.IsNotExist(err) {
		t.Fatal("diagnostic created lock")
	}
}
