package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCurrentSkewAndAtomicIdentityToken(t *testing.T) {
	for _, remaining := range []time.Duration{AccessSkew + time.Second, AccessSkew, AccessSkew - time.Second, -time.Second} {
		t.Run(remaining.String(), func(t *testing.T) {
			s, _ := testStore(t)
			old := activeRecord(testNow.Add(remaining))
			seed(t, s, old)
			calls := 0
			m := manager(t, s, roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				loaded, e := s.Load()
				if e != nil || loaded.State != "unusable" {
					t.Fatal("refresh side effect before durable intent")
				}
				return reply(200, tokenJSON("account-test", "refresh-replacement", testNow.Add(time.Hour))), nil
			}), nil)
			got, e := m.Current(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			want := 0
			if remaining <= AccessSkew {
				want = 1
			}
			if calls != want || got.Identity.Account != "account-test" || got.Identity.Epoch != 1 || got.Identity.Provider != "codex" {
				t.Fatal("wrong skew or atomic identity")
			}
			loaded, e := s.Load()
			if e != nil || got.AccessToken != loaded.Access || loaded.Generation != uint64(1+want) || loaded.AuthEpoch != 1 {
				t.Fatal("unpersisted credential published")
			}
		})
	}
}
func TestConcurrentRefreshCancelledWaiterDoesNotCancelRotation(t *testing.T) {
	s, _ := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Second)))
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	m := manager(t, s, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) != 1 {
			t.Error("duplicate rotation")
		}
		close(entered)
		select {
		case <-release:
			return reply(200, tokenJSON("account-test", "refresh-replacement", testNow.Add(time.Hour))), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	}), nil)
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, e := m.Current(ctx); first <- e }()
	<-entered
	const n = 32
	results := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := m.Current(context.Background())
			if e == nil && c.Identity.Epoch != 1 {
				e = errors.New("bad epoch")
			}
			results <- e
		}()
	}
	cancel()
	if e := <-first; !errors.Is(e, context.Canceled) {
		t.Fatal("waiter cancellation not honored", e)
	}
	close(release)
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("refresh not coalesced")
	}
}
func TestAmbiguousRefreshFailuresPoisonRestart(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		transport bool
	}{
		{name: "transport-timeout", transport: true}, {name: "invalid-grant", status: 400, body: `{"error":"invalid_grant"}`}, {name: "missing-replacement", status: 200, body: `{"access_token":"synthetic","expires_in":3600}`}, {name: "unchanged-replacement", status: 200, body: tokenJSON("account-test", "refresh-original", testNow.Add(time.Hour))}, {name: "account-change", status: 200, body: tokenJSON("different-account", "refresh-replacement", testNow.Add(time.Hour))}, {name: "malformed", status: 200, body: `{`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, path := testStore(t)
			seed(t, s, activeRecord(testNow.Add(time.Second)))
			calls := 0
			m := manager(t, s, roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if tc.transport {
					return nil, errors.New("timeout with synthetic-secret")
				}
				return reply(tc.status, tc.body), nil
			}), nil)
			if _, e := m.Current(context.Background()); e != ErrReauth || strings.Contains(e.Error(), "synthetic-secret") {
				t.Fatal("failure not explicit/redacted", e)
			}
			if _, e := m.Current(context.Background()); e == nil || calls != 1 {
				t.Fatal("old token retried")
			}
			m.Close()
			again, e := OpenStore(path)
			if e != nil {
				t.Fatal(e)
			}
			m2 := manager(t, again, roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("refresh retried after ambiguous restart")
				return nil, ErrOAuth
			}), nil)
			if _, e := m2.Current(context.Background()); e != ErrReauth {
				t.Fatal("restart did not fail closed", e)
			}
			status := m2.Status()
			if status.LoggedIn || !status.ReauthenticationRequired {
				t.Fatal("bad poisoned status")
			}
		})
	}
}
func TestRefreshPersistenceFaultsAndNoExternalSideEffectsBeforeIntent(t *testing.T) {
	for _, target := range []string{"intent.json", "record.json"} {
		for _, stage := range []string{"write", "fsync", "rename", "dirsync"} {
			t.Run(target+stage, func(t *testing.T) {
				s, path := testStore(t)
				seed(t, s, activeRecord(testNow.Add(time.Second)))
				s.fault = func(op string) error {
					if op == target+"."+stage {
						return ErrStore
					}
					return nil
				}
				var calls int
				m := manager(t, s, roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return reply(200, tokenJSON("account-test", "replacement", testNow.Add(time.Hour))), nil
				}), nil)
				if _, e := m.Current(context.Background()); e == nil {
					t.Fatal("replacement exposed after save fault")
				}
				if target == "intent.json" && calls != 0 {
					t.Fatal("external refresh before intent durability")
				}
				if target == "record.json" && calls != 1 {
					t.Fatal("wrong refresh count")
				}
				m.Close()
				again, e := OpenStore(path)
				if e != nil {
					t.Fatal(e)
				}
				defer again.Close()
				loaded, e := again.Load()
				if e != nil {
					t.Fatal(e)
				}
				if target == "record.json" && loaded.State != "unusable" {
					t.Fatal("failed rotation became usable on restart")
				}
			})
		}
	}
}
func TestPersistenceCompletesAfterCallerCancellation(t *testing.T) {
	s, path := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Second)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	s.fault = func(op string) error {
		if op == "record.json.write" {
			close(entered)
			<-release
		}
		return nil
	}
	m := manager(t, s, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(200, tokenJSON("account-test", "replacement", testNow.Add(time.Hour))), nil
	}), nil)
	first := make(chan error, 1)
	go func() { _, e := m.Current(ctx); first <- e }()
	<-entered
	cancel()
	if e := <-first; e != context.Canceled {
		t.Fatal("waiter did not cancel")
	}
	close(release)
	// Current joins the same finite persistence flight; no sleep/polling needed.
	c, e := m.Current(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	m.Close()
	again, e := OpenStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	saved, e := again.Load()
	if e != nil || saved.Refresh != "replacement" || saved.Access != c.AccessToken {
		t.Fatal("rotation lost after caller cancellation")
	}
}
func TestCloseCancelsRefreshWithoutStrandingWaiters(t *testing.T) {
	s, path := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Second)))
	entered := make(chan struct{})
	m := manager(t, s, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	}), nil)
	done := make(chan error, 1)
	go func() { _, e := m.Current(context.Background()); done <- e }()
	<-entered
	if e := m.Close(); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e == nil {
		t.Fatal("closed refresh returned credentials")
	}
	again, e := OpenStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	r, e := again.Load()
	if e != nil || r.State != "unusable" {
		t.Fatal("cancelled external rotation reused on restart")
	}
}
func loginTransport(t *testing.T) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			return reply(200, `{"device_auth_id":"device-test","user_code":"TEST-CODE","interval":1}`), nil
		case "/api/accounts/deviceauth/token":
			return reply(200, `{"authorization_code":"auth-code","code_verifier":"`+strings.Repeat("v", 43)+`"}`), nil
		case "/oauth/token":
			return reply(200, tokenJSON("account-test", "fresh-login", testNow.Add(time.Hour))), nil
		}
		t.Error("unexpected endpoint")
		return nil, ErrOAuth
	})
}
func TestSameAccountReauthenticationEpochLogoutAndCallbacks(t *testing.T) {
	s, _ := testStore(t)
	seed(t, s, activeRecord(testNow.Add(time.Hour)))
	var m *Manager
	var states []Status
	m = manager(t, s, loginTransport(t), func() {
		if m != nil {
			states = append(states, m.Status())
		}
	})
	if e := m.Login(context.Background(), func(DevicePrompt) error { return nil }); e != nil {
		t.Fatal(e)
	}
	c, e := m.Current(context.Background())
	if e != nil || c.Identity.Epoch != 2 {
		t.Fatal("same account reused epoch")
	}
	if len(states) != 2 || !states[0].ReauthenticationRequired || !states[1].LoggedIn {
		t.Fatal("callback not after state publication")
	}
	if e := m.Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	r, e := s.Load()
	if e != nil || r.State != "logged_out" || r.Access != "" || r.Refresh != "" || r.Generation != 3 || r.AuthEpoch != 2 {
		t.Fatal("logout tombstone invalid")
	}
	if _, e := m.Current(context.Background()); e == nil {
		t.Fatal("logged out credentials reused")
	}
	if e := m.Login(context.Background(), func(DevicePrompt) error { return nil }); e != nil {
		t.Fatal(e)
	}
	c, e = m.Current(context.Background())
	if e != nil || c.Identity.Epoch != 3 {
		t.Fatal("logout/login epoch not monotonic")
	}
}
func TestCounterOverflowAndRecordRedaction(t *testing.T) {
	old := activeRecord(testNow.Add(time.Hour))
	old.Generation = math.MaxUint64
	if _, e := next(old, false, testNow); e == nil {
		t.Fatal("generation overflow")
	}
	old.AuthEpoch = math.MaxUint64
	if _, e := next(old, true, testNow); e == nil {
		t.Fatal("epoch overflow")
	}
	secret := "synthetic-private-marker"
	r := activeRecord(testNow.Add(time.Hour))
	r.Access = secret
	r.Refresh = secret
	r.Account = secret
	id := Identity{Account: secret}
	cred := Credential{Identity: id, AccessToken: secret}
	prompt := DevicePrompt{secret}
	device := deviceAuthorization{ID: secret, Code: secret}
	code := authorizationCode{Code: secret, Verifier: secret}
	token := tokenResponse{Access: secret, Refresh: secret}
	var metadata accessMetadata
	metadata.Auth.Account = secret
	for _, v := range []any{r, &r, id, &id, cred, &cred, prompt, &prompt, device, &device, code, &code, token, &token, metadata, &metadata, &Manager{record: r}, &DiskStore{}} {
		var output bytes.Buffer
		slog.New(slog.NewJSONHandler(&output, nil)).Info("redaction", "value", v)
		if strings.Contains(output.String(), secret) {
			t.Fatal("secret slog disclosure")
		}
		expected := fmt.Sprint(v)
		for _, format := range []string{"%d", "%x", "%q", "%s", "%v", "%+v", "%#v", "%020d", "%.2s", "%+x", "%Z"} {
			got := fmt.Sprintf(format, v)
			if strings.Contains(got, secret) || got != expected {
				t.Fatal("formatter did not emit only its constant redaction label")
			}
		}
	}
	// Log/format protection must not hide the actual private-store JSON fields.
	encoded, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	var stored struct {
		Access  string `json:"access"`
		Refresh string `json:"refresh"`
		Account string `json:"account"`
	}
	if json.Unmarshal(encoded, &stored) != nil || stored.Access != secret || stored.Refresh != secret || stored.Account != secret {
		t.Fatal("intentional store serialization was redacted")
	}
}
