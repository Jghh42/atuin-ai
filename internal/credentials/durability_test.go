package credentials

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestFailedBeginCanBeRecoveredByExplicitLogin(t *testing.T) {
	s, _ := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Second)))
	s.fault = func(op string) error {
		if op == "intent.json.write" {
			return ErrStore
		}
		return nil
	}
	var invalidations int
	m := manager(t, s, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("refresh sent before failed intent")
		return nil, ErrOAuth
	}), func() { invalidations++ })
	if _, e := m.Current(context.Background()); e == nil {
		t.Fatal("failed begin exposed credential")
	}
	if invalidations != 1 {
		t.Fatal("failed refresh did not synchronously invalidate")
	}
	s.fault = nil
	m.oauth.Transport = loginTransport(t)
	if e := m.Login(context.Background(), func(DevicePrompt) error { return nil }); e != nil {
		t.Fatal("explicit login could not recover reserved counter gap", e)
	}
	r, e := s.Load()
	if e != nil || r.Generation != 3 || r.AuthEpoch != 2 {
		t.Fatal("reserved counters lost")
	}
}
func TestSuccessfulRotationPreservesEpochAndCatalog(t *testing.T) {
	s, _ := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Second)))
	var invalidations int
	m := manager(t, s, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(200, tokenJSON("account-test", "replacement", testNow.Add(time.Hour))), nil
	}), func() { invalidations++ })
	c, e := m.Current(context.Background())
	if e != nil || c.Identity.Epoch != 1 || invalidations != 0 {
		t.Fatal("ordinary rotation changed catalog lifecycle", e)
	}
}
func TestFinalIntentCleanupFaultNeverRestoresOldRefresh(t *testing.T) {
	for _, stage := range []string{"intent.unlink", "intent.dirsync"} {
		t.Run(stage, func(t *testing.T) {
			s, path := testStore(t)
			seed(t, s, activeRecord(testNow.Add(time.Second)))
			s.fault = func(op string) error {
				if op == stage {
					return ErrStore
				}
				return nil
			}
			m := manager(t, s, roundTripFunc(func(*http.Request) (*http.Response, error) {
				return reply(200, tokenJSON("account-test", "replacement", testNow.Add(time.Hour))), nil
			}), nil)
			if _, e := m.Current(context.Background()); e == nil {
				t.Fatal("failed transaction exposed a token")
			}
			m.Close()
			again, e := OpenStore(path)
			if e != nil {
				t.Fatal(e)
			}
			defer again.Close()
			r, e := again.Load()
			if e != nil {
				t.Fatal(e)
			}
			// After unlink-dirsync failure either the durable replacement or the old
			// intent can survive a power loss. Neither permits use of the old token.
			if r.State == "active" && (r.Refresh != "replacement" || r.Generation != 2) {
				t.Fatal("old refresh token restored")
			}
			if stage == "intent.unlink" && r.State != "unusable" {
				t.Fatal("retained intent ignored")
			}
		})
	}
}
func TestFailedLoginPersistenceRemainsUnusableAcrossRestart(t *testing.T) {
	s, path := testStore(t)
	s.fault = func(op string) error {
		if op == "record.json.dirsync" {
			return ErrStore
		}
		return nil
	}
	m := manager(t, s, loginTransport(t), nil)
	if e := m.Login(context.Background(), func(DevicePrompt) error { return nil }); e == nil {
		t.Fatal("unpersisted login reported success")
	}
	if _, e := m.Current(context.Background()); e == nil {
		t.Fatal("unpersisted login exposed")
	}
	m.Close()
	again, e := OpenStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	r, e := again.Load()
	if e != nil || r.State != "unusable" {
		t.Fatal("failed login became usable on restart")
	}
}
func TestLoginPersistenceSurvivesCallerCancellation(t *testing.T) {
	s, _ := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.fault = func(op string) error {
		if op == "record.json.write" {
			cancel()
		}
		return nil
	}
	m := manager(t, s, loginTransport(t), nil)
	if e := m.Login(ctx, func(DevicePrompt) error { return nil }); e != nil {
		t.Fatal("issued login lost after cancellation", e)
	}
	if _, e := m.Current(context.Background()); e != nil {
		t.Fatal("durable login unavailable", e)
	}
}
func TestLogoutPersistenceFailureDoesNotClaimSuccess(t *testing.T) {
	s, path := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Hour)))
	s.fault = func(op string) error {
		if op == "record.json.fsync" {
			return ErrStore
		}
		return nil
	}
	m := manager(t, s, nil, nil)
	if e := m.Logout(context.Background()); e == nil {
		t.Fatal("failed secret deletion reported success")
	}
	if _, e := m.Current(context.Background()); e == nil {
		t.Fatal("failed logout kept usable in-memory secrets")
	}
	m.Close()
	again, e := OpenStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	r, e := again.Load()
	if e != nil || r.State != "unusable" {
		t.Fatal("failed logout reused secret record")
	}
}
