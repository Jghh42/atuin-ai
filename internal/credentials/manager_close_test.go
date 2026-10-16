package credentials

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Both persistence and the final storage Close have explicit gates. In
// particular, source unavailability is not evidence that the flock is released.
type gatedCloseStorage struct {
	Storage
	entered chan struct{}
	release chan struct{}
	result  error
	calls   atomic.Int32
}

func (s *gatedCloseStorage) Close() error {
	s.calls.Add(1)
	close(s.entered)
	<-s.release
	if e := s.Storage.Close(); e != nil {
		return e
	}
	return s.result
}
func awaitCloseTest[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown handshake timed out")
		var zero T
		return zero
	}
}
func assertClosePending(t *testing.T, first, second <-chan error) {
	t.Helper()
	// The gates establish the blocked work; this finite guard checks the
	// forbidden early return, rather than using a sleep to start cancellation.
	select {
	case <-first:
		t.Fatal("first Close returned before shutdown completed")
	case <-second:
		t.Fatal("concurrent Close returned before shutdown completed")
	case <-time.After(25 * time.Millisecond):
	}
}
func TestConcurrentCloseJoinsPersistenceStoreReleaseAndSharedResult(t *testing.T) {
	for _, closeErr := range []error{nil, errors.New("injected storage close failure")} {
		name := "success"
		if closeErr != nil {
			name = "error"
		}
		t.Run(name, func(t *testing.T) {
			s, path := testStore(t)
			seed(t, s, activeRecord(testNow.Add(time.Second)))
			persistEntered, persistRelease := make(chan struct{}), make(chan struct{})
			var releasePersist sync.Once
			s.fault = func(stage string) error {
				if stage == "record.json.write" {
					close(persistEntered)
					<-persistRelease
				}
				return nil
			}
			storage := &gatedCloseStorage{Storage: s, entered: make(chan struct{}), release: make(chan struct{}), result: closeErr}
			shutdownStarted := make(chan struct{})
			var shutdownNotice sync.Once
			m := manager(t, storage, roundTripFunc(func(*http.Request) (*http.Response, error) {
				return reply(200, tokenJSON("account-test", "replacement", testNow.Add(time.Hour))), nil
			}), func() { shutdownNotice.Do(func() { close(shutdownStarted) }) })
			var releaseStore sync.Once
			t.Cleanup(func() {
				releasePersist.Do(func() { close(persistRelease) })
				releaseStore.Do(func() { close(storage.release) })
			})
			currentDone := make(chan error, 1)
			go func() { _, e := m.Current(context.Background()); currentDone <- e }()
			awaitCloseTest(t, persistEntered)
			first, second := make(chan error, 1), make(chan error, 1)
			go func() { first <- m.Close() }()
			awaitCloseTest(t, shutdownStarted)
			secondStarted := make(chan struct{})
			go func() { close(secondStarted); second <- m.Close() }()
			awaitCloseTest(t, secondStarted)
			assertClosePending(t, first, second)
			if _, e := m.Current(context.Background()); e != ErrUnavailable {
				t.Fatal("closing source remained available", e)
			}
			if m.Status().LoggedIn {
				t.Fatal("closing status claimed login availability")
			}
			m.mu.Lock()
			closing, closed := m.closing, m.closed
			m.mu.Unlock()
			if !closing || closed {
				t.Fatal("closing and fully closed states conflated")
			}
			if again, e := OpenStore(path); e == nil {
				again.Close()
				t.Fatal("state reopened while persistence still held")
			}
			releasePersist.Do(func() { close(persistRelease) })
			awaitCloseTest(t, storage.entered)
			assertClosePending(t, first, second)
			if again, e := OpenStore(path); e == nil {
				again.Close()
				t.Fatal("state reopened before storage Close completed")
			}
			releaseStore.Do(func() { close(storage.release) })
			if e := awaitCloseTest(t, first); e != closeErr {
				t.Fatal("first Close lost store result", e)
			}
			if e := awaitCloseTest(t, second); e != closeErr {
				t.Fatal("concurrent Close lost shared result", e)
			}
			if e := m.Close(); e != closeErr {
				t.Fatal("later Close lost shared result", e)
			}
			if e := awaitCloseTest(t, currentDone); e == nil {
				t.Fatal("closing waiter received a credential")
			}
			if storage.calls.Load() != 1 {
				t.Fatal("storage Close was repeated")
			}
			m.mu.Lock()
			closing, closed = m.closing, m.closed
			m.mu.Unlock()
			if closing || !closed {
				t.Fatal("shutdown completion was not published")
			}
			again, e := OpenStore(path)
			if e != nil {
				t.Fatal("completed shutdown retained flock", e)
			}
			defer again.Close()
			saved, e := again.Load()
			if e != nil || saved.State != "active" || saved.Generation != 2 || saved.Refresh != "replacement" {
				t.Fatal("shutdown lost detached replacement persistence", e)
			}
		})
	}
}
