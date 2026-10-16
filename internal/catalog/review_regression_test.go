package catalog

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func TestCompletedStaleFlightRejectedAfterHardFailure(t *testing.T) {
	for _, hard := range []error{ErrUnauthorized, ErrInvalid, ErrOverride} {
		t.Run(hard.Error(), func(t *testing.T) {
			c, f := setup()
			ctx := context.Background()
			if _, err := c.Get(ctx); err != nil {
				t.Fatal(err)
			}
			f.now = f.now.Add(FreshFor)
			f.errorFetch = ErrTransient
			// The first waiter's refresh completes transiently with stale data, but its
			// consumption is deliberately delayed at the same boundary used by Get.
			c.mu.Lock()
			stale := &flight{done: make(chan struct{}), cancel: func() {}, waiters: 1, revision: c.revision}
			c.active = stale
			c.mu.Unlock()
			completed, consume := make(chan struct{}), make(chan struct{})
			type result struct {
				snapshot Snapshot
				err      error
			}
			delayed := make(chan result, 1)
			go func() {
				<-stale.done
				close(completed)
				<-consume
				s, err := c.flightResult(f.cred.Identity, stale)
				delayed <- result{s, err}
			}()
			c.refresh(ctx, f.cred, stale)
			<-completed
			if stale.snapshot.response.Default == "" || stale.err != nil {
				t.Fatal("fixture did not complete stale")
			}
			f.errorFetch = hard
			if _, err := c.Get(ctx); !errors.Is(err, hard) {
				t.Fatal("new failing flight lost its error", err)
			}
			close(consume)
			old := <-delayed
			if old.err == nil || old.snapshot.response.Default != "" {
				t.Fatal("delayed waiter returned stale data after hard invalidation", old.err)
			}
		})
	}
}

func TestObsoleteCredentialLookupErrorCannotEraseReauthenticatedCache(t *testing.T) {
	c, f := setup()
	entered, release := make(chan struct{}), make(chan struct{})
	var lookups atomic.Int32
	newCredential := f.cred
	newCredential.Identity.Account = "new-account"
	newCredential.Identity.Epoch++
	c.source = credentials.SourceFunc(func(ctx context.Context) (credentials.Credential, error) {
		if lookups.Add(1) == 1 {
			close(entered)
			<-release
			return credentials.Credential{}, ErrUnauthorized
		}
		return newCredential, nil
	})
	oldDone := make(chan error, 1)
	go func() { _, err := c.Get(context.Background()); oldDone <- err }()
	<-entered
	// Publish new source state, then synchronously invalidate, as lifecycle
	// writers are required to do before reporting completed reauthentication.
	c.Invalidate()
	if _, err := c.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := f.calls
	close(release)
	if err := <-oldDone; !errors.Is(err, ErrUnavailable) {
		t.Fatal("obsolete lookup was applied", err)
	}
	if _, err := c.Get(context.Background()); err != nil || f.calls != calls {
		t.Fatal("obsolete lookup erased new validated cache", err, f.calls, calls)
	}
}
