package catalog

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func entry(id string, p int32) Entry { return Entry{ID: id, Name: id, Visibility: "list", Priority: p} }

func TestBundledExactOrderDefaultOverridesAndCopies(t *testing.T) {
	for _, override := range []string{"", "gpt-6.1-sol", "gpt-6-luna"} {
		s, err := BuildBundled(override)
		if err != nil {
			t.Fatal(err)
		}
		want := override
		if want == "" {
			want = "gpt-6.1-sol"
		}
		r := s.Response()
		if r.Default != want || !reflect.DeepEqual(r.Models, []Model{
			{"gpt-6.1-sol", "Sol", "Native Codex Sol"},
			{"gpt-6-luna", "Luna", "Native Codex Luna (live validation pending)"},
		}) {
			t.Fatal(r)
		}
		if s.Resolve("unknown") != want || s.Resolve("gpt-6-luna") != "gpt-6-luna" {
			t.Fatal("resolution")
		}
		r.Models[0].Alias = "mutated"
		r.Default = "mutated"
		if s.Response().Models[0].Alias != "gpt-6.1-sol" {
			t.Fatal("mutable response")
		}
	}
	for _, id := range []string{"unknown", "gpt-6-sol", "GPT-6-LUNA", " gpt-6-luna"} {
		if _, err := BuildBundled(id); !errors.Is(err, ErrOverride) {
			t.Fatal(id, err)
		}
	}
}

func TestHistoricalProofIndependentOfBundledMembership(t *testing.T) {
	luna := []Entry{entry("gpt-6-luna", 0)}
	if e := InspectEligibility(luna); e.ProvenPresent || e.EligibleCount != 0 {
		t.Fatal(e)
	}
	if _, err := Build(luna, ""); err == nil {
		t.Fatal("Luna acquired historical proof")
	}
	if _, err := Build([]Entry{entry("gpt-6.1-sol", 0), entry("gpt-6-luna", 1)}, "gpt-6-luna"); !errors.Is(err, ErrOverride) {
		t.Fatal(err)
	}
}

type fixture struct {
	mu        sync.Mutex
	cred      credentials.Credential
	sourceErr error
	calls     int
}

func (f *fixture) current(context.Context) (credentials.Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.cred, f.sourceErr
}
func setup() (*Cache, *fixture) {
	f := &fixture{cred: credentials.Credential{Identity: credentials.Identity{Provider: "codex", Account: "a", Epoch: 1}, AccessToken: "fake"}}
	return New(credentials.SourceFunc(f.current), ""), f
}

func TestCredentialPreflightEveryLookupAndIdentityPinning(t *testing.T) {
	c, f := setup()
	for _, change := range []string{"initial", "account", "reauth", "rotation"} {
		f.mu.Lock()
		switch change {
		case "account":
			f.cred.Identity.Account = "b"
		case "reauth":
			f.cred.Identity.Epoch++
		case "rotation":
			f.cred.AccessToken = "rotated"
		}
		identity := f.cred.Identity
		f.mu.Unlock()
		s, err := c.Get(context.Background())
		if err != nil || s.Identity() != identity {
			t.Fatal(change, err, s.Identity())
		}
	}
	if f.calls != 4 {
		t.Fatal("skipped preflight", f.calls)
	}
	f.sourceErr = ErrUnauthorized
	if _, err := c.Get(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if c.snapshot.response.Default != "" {
		t.Fatal("cached credentials survived logout")
	}
}
func TestNoCredentialsNoCatalog(t *testing.T) {
	for _, cred := range []credentials.Credential{{}, {Identity: credentials.Identity{Provider: "other", Account: "a"}, AccessToken: "fake"}, {Identity: credentials.Identity{Provider: "codex"}, AccessToken: "fake"}, {Identity: credentials.Identity{Provider: "codex", Account: "a"}}} {
		c := New(credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return cred, nil }), "")
		if _, err := c.Get(context.Background()); err == nil {
			t.Fatal("credentialless catalog")
		}
	}
	if _, err := New(nil, "").Get(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	c, _ := setup()
	c.override = "unknown"
	if _, err := c.Get(context.Background()); !errors.Is(err, ErrOverride) {
		t.Fatal(err)
	}
}
func TestConcurrentLocalLookupAndCancellation(t *testing.T) {
	c, f := setup()
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := c.Get(context.Background())
			if err != nil || len(s.Response().Models) != 2 {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if f.calls != 32 {
		t.Fatal(f.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Get(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestInvalidateDuringCredentialLookup(t *testing.T) {
	for _, sourceErr := range []error{nil, ErrUnauthorized} {
		c, f := setup()
		entered, release := make(chan struct{}), make(chan struct{})
		var calls atomic.Int32
		old := f.cred
		c.source = credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
				return old, sourceErr
			}
			next := old
			next.Identity.Epoch++
			return next, nil
		})
		done := make(chan error, 1)
		go func() { _, err := c.Get(context.Background()); done <- err }()
		<-entered
		c.Invalidate()
		s, err := c.Get(context.Background())
		if err != nil || s.Identity().Epoch != 2 {
			t.Fatal(err)
		}
		close(release)
		if err := <-done; !errors.Is(err, ErrUnavailable) {
			t.Fatal("obsolete publication", err)
		}
		s, err = c.Get(context.Background())
		if err != nil || s.Identity().Epoch != 2 {
			t.Fatal("obsolete lookup erased new snapshot", err)
		}
	}
}
