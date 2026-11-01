package continuation

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type setupTimeContext struct {
	context.Context
	hook func()
}

func (c setupTimeContext) Value(key any) any { c.hook(); return c.Context.Value(key) }

// Expiry must also be checked after bounded cancellation-linkage setup, not
// only before it. The hook advances fake time at the context-parent inspection
// during that setup, after the final directory sync has already succeeded.
func TestPublicationRechecksExpiryAfterContextLinkageSetup(t *testing.T) {
	f := newFixture(t)
	l, scope := f.lease(t, context.Background(), nil)
	r := f.build(t, l, scope, nil, nil)
	r.expires = f.now.Load() + 1000
	r.segments[0].expires = r.expires
	var armed atomic.Bool
	l.linked = setupTimeContext{Context: l.linked, hook: func() {
		if armed.Load() {
			f.now.Store(r.expires)
		}
	}}
	f.store.fault = func(step string) error {
		if step == "after-dirsync" {
			armed.Store(true)
		}
		return nil
	}
	called := false
	durable, e := f.publish(l, context.Background(), r, func(context.Context) error { called = true; return nil })
	if f.now.Load() != r.expires {
		t.Fatal("context setup seam was not reached")
	}
	typed, ok := e.(Error)
	if !durable || called || !ok || typed.Code() != Expired {
		t.Fatal("post-setup expiry granted callback authority", e)
	}
}

// A concurrent local filesystem operation can hold store.mu while Close has
// already canceled manager life. The publication callback's Done must wake from
// that DIRECT lifetime source, not wait until Suspend can acquire store.mu and
// propagate scope cancellation. No callback calls back into Manager.
func TestPublicationDoneObservesManagerLifeBeforeScopeSuspendCanLock(t *testing.T) {
	f := newFixture(t)
	l, scope := f.lease(t, context.Background(), nil)
	r := f.build(t, l, scope, nil, nil)
	entered, ioLocked := make(chan struct{}), make(chan struct{})
	notLinked := errors.New("publication Done was not linked to manager life")
	type outcome struct {
		durable bool
		err     error
	}
	published := make(chan outcome, 1)
	go func() {
		d, e := f.publish(l, context.Background(), r, func(ctx context.Context) error {
			close(entered)
			<-ioLocked
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return notLinked
			}
		})
		published <- outcome{d, e}
	}()
	<-entered
	f.store.mu.Lock()
	close(ioLocked)
	closed := make(chan error, 1)
	go func() { closed <- f.manager.Close() }()
	got := <-published
	f.store.mu.Unlock()
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
	if got.err == notLinked {
		t.Fatal("publication Done waited for store mutex instead of manager life")
	}
	if !got.durable || got.err == nil {
		t.Fatal("lifetime cancellation or durable record outcome lost")
	}
}
