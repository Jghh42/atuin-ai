package continuation

import (
	"context"
	"time"
)

// Explicit forwarding avoids embedding an exported Context field: custom value
// contexts may contain credentials or arbitrary private request values.
func (c leaseContext) Deadline() (time.Time, bool) {
	if c.leaseContextPayload == nil || c.context == nil {
		return time.Time{}, false
	}
	return c.context.Deadline()
}
func (c leaseContext) Done() <-chan struct{} {
	if c.leaseContextPayload == nil || c.context == nil {
		return closedContextDone
	}
	return c.context.Done()
}
func (c leaseContext) Value(key any) any {
	if c.leaseContextPayload == nil || c.context == nil {
		return nil
	}
	return c.context.Value(key)
}
func (c emitContext) Deadline() (time.Time, bool) {
	if c.emitContextPayload == nil || c.context == nil {
		return time.Time{}, false
	}
	return c.context.Deadline()
}
func (c emitContext) Done() <-chan struct{} {
	if c.emitContextPayload == nil || c.context == nil {
		return closedContextDone
	}
	return c.context.Done()
}
func (c emitContext) Value(key any) any {
	if c.emitContextPayload == nil || c.context == nil {
		return nil
	}
	return c.context.Value(key)
}

var closedContextDone = func() <-chan struct{} { c := make(chan struct{}); close(c); return c }()
var _ context.Context = leaseContext{}
var _ context.Context = emitContext{}
