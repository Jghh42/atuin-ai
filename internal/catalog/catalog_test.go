package catalog

import (
	"context"
	"errors"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func entry(id string, p int32) Entry { return Entry{ID: id, Name: id, Visibility: "list", Priority: p} }
func TestSelection(t *testing.T) {
	all := func(string) bool { return true }
	for _, tc := range []struct {
		name           string
		entries        []Entry
		override, want string
		err            error
	}{
		{"priority numeric not lexical", []Entry{entry("gpt-10-sol", 10), entry("gpt-2-sol", 2)}, "", "gpt-2-sol", nil},
		{"sol preference", []Entry{entry("other", 0), entry("gpt-6.1-sol", 100)}, "", "gpt-6.1-sol", nil},
		{"tie", []Entry{entry("b", 2), entry("a", 2)}, "", "a", nil},
		{"no sol", []Entry{entry("b", 10), entry("a", 2)}, "", "a", nil},
		{"override", []Entry{entry("a", 0), entry("b", 1)}, "b", "b", nil},
		{"invalid override", []Entry{entry("a", 0)}, "b", "", ErrOverride},
		{"empty", nil, "", "", ErrInvalid},
		{"duplicates", []Entry{entry("a", 0), entry("a", 1)}, "", "", ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := selectSnapshot(tc.entries, tc.override, all)
			if !errors.Is(err, tc.err) {
				t.Fatal(err)
			}
			if err == nil && s.Resolve("missing") != tc.want {
				t.Fatal(s.Response())
			}
		})
	}
	for _, m := range []Entry{{ID: "x", Name: "x", Visibility: "hide"}, {ID: "x", Name: "x", Visibility: "none"}, entry("unknown", 0), {ID: "x", Name: strings.Repeat("x", 257), Visibility: "list"}, entry("bad id", 0)} {
		if _, err := selectSnapshot([]Entry{m}, "", func(id string) bool { return id == "gpt-6.1-sol" }); err == nil {
			t.Fatal(m)
		}
	}
	if _, err := selectSnapshot(make([]Entry, 257), "", all); err == nil {
		t.Fatal("unbounded")
	}
}

type fetchFunc func(context.Context, credentials.Credential) ([]Entry, error)

func (f fetchFunc) Fetch(ctx context.Context, c credentials.Credential) ([]Entry, error) {
	return f(ctx, c)
}

type fixture struct {
	mu                    sync.Mutex
	now                   time.Time
	cred                  credentials.Credential
	sourceErr, errorFetch error
	calls                 int
}

func (f *fixture) current(context.Context) (credentials.Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cred, f.sourceErr
}
func (f *fixture) fetch(context.Context, credentials.Credential) ([]Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return []Entry{entry("gpt-6.1-sol", 0)}, f.errorFetch
}
func setup() (*Cache, *fixture) {
	f := &fixture{now: time.Unix(10000, 0), cred: credentials.Credential{Identity: credentials.Identity{Provider: "codex", Account: "a", Epoch: 1}, AccessToken: "fake"}}
	return New(credentials.SourceFunc(f.current), fetchFunc(f.fetch), "", func() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.now }), f
}
func TestTTLStaleAndImmutable(t *testing.T) {
	c, f := setup()
	ctx := context.Background()
	s, err := c.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := s.Response()
	r.Models[0].Alias = "mutated"
	r.Default = "mutated"
	if s.Resolve("unknown") != "gpt-6.1-sol" {
		t.Fatal("mutable")
	}
	if _, err = c.Get(ctx); err != nil || f.calls != 1 {
		t.Fatal(err, f.calls)
	}
	f.now = f.now.Add(FreshFor)
	f.errorFetch = ErrTransient
	if _, err = c.Get(ctx); err != nil || f.calls != 2 {
		t.Fatal(err, f.calls)
	}
	f.now = f.now.Add(StaleFor - FreshFor)
	if _, err = c.Get(ctx); err != nil {
		t.Fatal("exact stale bound", err)
	}
	f.now = f.now.Add(time.Nanosecond)
	if _, err = c.Get(ctx); err == nil {
		t.Fatal("too stale")
	}
}
func TestHardFailureAccountAndLogout(t *testing.T) {
	for _, failure := range []error{ErrUnauthorized, ErrInvalid, ErrOverride} {
		c, f := setup()
		ctx := context.Background()
		if _, err := c.Get(ctx); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(FreshFor)
		f.errorFetch = failure
		if _, err := c.Get(ctx); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		f.errorFetch = ErrTransient
		if _, err := c.Get(ctx); err == nil {
			t.Fatal("revived hard failure")
		}
	}
	for _, change := range []string{"account", "reauth", "logout"} {
		t.Run(change, func(t *testing.T) {
			c, f := setup()
			ctx := context.Background()
			if _, err := c.Get(ctx); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "account":
				f.cred.Identity.Account = "b"
			case "reauth":
				f.cred.Identity.Epoch++
			case "logout":
				f.sourceErr = ErrUnauthorized
			}
			f.errorFetch = ErrTransient
			if _, err := c.Get(ctx); err == nil {
				t.Fatal("reused stale credentials")
			}
		})
	}
}
func TestCoalescingAndCancellation(t *testing.T) {
	c, _ := setup()
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c.fetcher = fetchFunc(func(ctx context.Context, _ credentials.Credential) ([]Entry, error) {
		calls.Add(1)
		close(entered)
		select {
		case <-release:
			return []Entry{entry("gpt-6.1-sol", 0)}, nil
		case <-ctx.Done():
			return nil, ErrTransient
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, e := c.Get(ctx); first <- e }()
	<-entered
	const n = 12
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		waiters := c.active.waiters
		c.mu.Unlock()
		if waiters == n+1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not coalesced")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
func TestLastWaiterCancelsAndInvalidation(t *testing.T) {
	for _, invalidate := range []bool{false, true} {
		c, _ := setup()
		entered, stopped := make(chan struct{}), make(chan struct{})
		c.fetcher = fetchFunc(func(ctx context.Context, _ credentials.Credential) ([]Entry, error) {
			close(entered)
			<-ctx.Done()
			close(stopped)
			return nil, ErrTransient
		})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { _, err := c.Get(ctx); done <- err }()
		<-entered
		if invalidate {
			c.Invalidate()
		} else {
			cancel()
		}
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("fetch not cancelled")
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("success after cancellation")
			}
		case <-time.After(time.Second):
			t.Fatal("waiter stuck")
		}
		cancel()
	}
}
func TestInvalidateDuringCredentialLookup(t *testing.T) {
	c, f := setup()
	entered, release := make(chan struct{}), make(chan struct{})
	c.source = credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { close(entered); <-release; return f.cred, nil })
	done := make(chan error, 1)
	go func() { _, err := c.Get(context.Background()); done <- err }()
	<-entered
	c.Invalidate()
	close(release)
	if err := <-done; err == nil || f.calls != 0 {
		t.Fatal("old credential repopulated cache", err, f.calls)
	}
}

func TestNoCredentialsNoFetch(t *testing.T) {
	c, f := setup()
	f.sourceErr = ErrUnauthorized
	if _, err := c.Get(context.Background()); err == nil || f.calls != 0 {
		t.Fatal(err, f.calls)
	}
	c = New(nil, nil, "", nil)
	if _, err := c.Get(context.Background()); err == nil {
		t.Fatal("fake fallback")
	}
}
