package credentials

import (
	"context"
	"sync"
)

// operationGate is zero-safe. Waiters use their own goroutine, never a helper.
type operationGate struct {
	once  sync.Once
	token chan struct{}
}

func (g *operationGate) init() {
	g.once.Do(func() { g.token = make(chan struct{}, 1); g.token <- struct{}{} })
}
func (g *operationGate) Lock() { _ = g.LockContext(context.Background()) }
func (g *operationGate) LockContext(ctx context.Context) error {
	g.init()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.token:
	}
	if ctx.Err() != nil {
		g.Unlock()
		return ctx.Err()
	}
	return nil
}
func (g *operationGate) TryLock() bool {
	g.init()
	select {
	case <-g.token:
		return true
	default:
		return false
	}
}
func (g *operationGate) Unlock() { g.token <- struct{}{} }
