package continuation

import (
	"context"
	"crypto/rand"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

// Lease has no worker goroutine. done means ONLY native stream/sink cleanup
// has completed. It never means Publish/Close (which can await operation gate).
type leasePayload struct {
	store                               *Store
	original, scope, linked             context.Context
	cancel                              context.CancelFunc
	stop                                func() bool
	marked                              bool
	started, completed, revoked, closed bool // store.mu
	done                                chan struct{}
}
type leaseContextPayload struct {
	context context.Context
	l       *Lease
}

func (c leaseContext) Err() error {
	if c.leaseContextPayload == nil || c.l == nil {
		return context.Canceled
	}
	if e := c.l.original.Err(); e != nil {
		return e
	}
	if e := c.l.scope.Err(); e != nil {
		return e
	}
	return c.context.Err()
}
func (s *Store) Acquire(ctx context.Context, messages []atuin.Message) (*Lease, error) {
	if len(messages) > atuin.MaxMessages {
		return nil, failure(Limit)
	}
	_, marked, e := latest(messages)
	if e != nil {
		return nil, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.active || ctx.Err() != nil {
		return nil, failure(LifecycleChanged)
	}
	if len(s.leases) >= MaxScopes {
		return nil, failure(Limit)
	}
	linked, cancel := context.WithCancel(ctx)
	l := ownedPointer(newLease(leasePayload{store: s, original: ctx, scope: s.scope, linked: linked, cancel: cancel, marked: marked, done: make(chan struct{})}))
	l.stop = context.AfterFunc(s.scope, cancel)
	s.leases[l] = true
	return l, nil
}
func (l *Lease) Context() context.Context {
	if l == nil || l.leasePayload == nil {
		return leaseContext{}
	}
	return newLeaseContext(leaseContextPayload{l.linked, l})
}
func (l *Lease) checkLocked() error {
	if l.closed || l.revoked || l.store.closed || !l.store.active || l.original.Err() != nil || l.scope.Err() != nil || l.linked.Err() != nil {
		return failure(LifecycleChanged)
	}
	return nil
}
func (l *Lease) Check() error {
	if l == nil || l.leasePayload == nil || l.store == nil {
		return failure(LifecycleChanged)
	}
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	return l.checkLocked()
}
func (l *Lease) MarkStreamStarted() error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	if l.checkLocked() != nil || l.started {
		return failure(LifecycleChanged)
	}
	l.started = true
	return nil
}

// StreamDone MUST follow body.Close, watchdog join and callback return. It must
// precede operation-gate acquisition. It is idempotent on cleanup paths.
func (l *Lease) StreamDone() {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	if l.started && !l.completed {
		l.completed = true
		close(l.done)
	}
}
func (l *Lease) Close() {
	if l == nil || l.leasePayload == nil || l.store == nil {
		return
	}
	s := l.store
	s.mu.Lock()
	l.revoked = true
	l.cancel()
	l.stop()
	wait := l.started && !l.completed
	done := l.done
	s.mu.Unlock()
	if wait {
		<-done
	}
	s.mu.Lock()
	l.closed = true
	delete(s.leases, l)
	s.mu.Unlock()
}

// Build constructs a self-contained candidate, taking ownership of recipe and
// ancestry buffers. It persists high-water BEFORE issuing any protected ID.
// Validated calls must be executable (never terminal suggestions).
func (l *Lease) Build(scope Commitment, prefix Prefix, text TextExpectation, recipe TextRecipe, calls []Call, items []Item, ancestry []Segment) (Record, error) {
	s := l.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.checkLocked() != nil || s.key == nil {
		return newRecord(recordPayload{}), failure(LifecycleChanged)
	}
	if len(calls) == 0 || len(calls) > MaxCalls || len(ancestry) >= MaxSegments {
		return newRecord(recordPayload{}), failure(Limit)
	}
	if e := boundedInputs(ancestry, items, calls, recipe); e != nil {
		return newRecord(recordPayload{}), e
	}
	if scope.commitmentPayload == nil {
		return newRecord(recordPayload{}), failure(BindingMismatch)
	}
	ownedCalls := makeCall(len(calls))
	for j := range calls {
		ownedCalls[j] = cloneCall(calls[j])
	}
	calls = ownedCalls
	now, e := s.clock(l.original)
	if e != nil {
		return newRecord(recordPayload{}), e
	}
	if now > maxMillis-TTLMillis {
		return newRecord(recordPayload{}), failure(ClockInvalid)
	}
	var id SnapshotID = newSnapshotID(snapshotIDPayload{})
	if _, e := rand.Read(id.bytes[:]); e != nil {
		return newRecord(recordPayload{}), failure(Unavailable)
	}
	expires := now + TTLMillis
	for _, a := range ancestry {
		if now >= a.expires {
			return newRecord(recordPayload{}), failure(Expired)
		}
		if a.expires < expires {
			expires = a.expires
		}
	}
	// No filenames/provider IDs feed randomness; avoid snapshot/public collisions.
	for _, a := range ancestry {
		if sameSnapshot(a.snapshot, id) {
			return newRecord(recordPayload{}), failure(CommitFailed)
		}
	}
	seen := map[string]bool{}
	for j := range calls {
		c := &calls[j]
		if c.backend.value == "" || Reserved(c.backend.value) || c.name == "suggest_command" || seen[c.backend.value] {
			return newRecord(recordPayload{}), failure(ProjectionUnsupported)
		}
		seen[c.backend.value] = true
		found := false
		for _, i := range items {
			if i.kind == 2 && i.callID == c.backend.value && i.name == c.name {
				digest, e := s.key.arguments([]byte(i.arguments), true)
				if e != nil {
					return newRecord(recordPayload{}), e
				}
				c.publicArguments = digest
				found = true
				break
			}
		}
		if !found {
			return newRecord(recordPayload{}), failure(BindingMismatch)
		}
		c.handle, e = s.key.handle(id, uint8(j), scope)
		if e != nil {
			return newRecord(recordPayload{}), e
		}
	}
	segment := newSegment(segmentPayload{snapshot: id, created: now, expires: expires, prefix: prefix, text: text, recipe: recipe, calls: calls, items: items})
	segments := makeSegment(0, len(ancestry)+1)
	segments = append(segments, ancestry...)
	segments = append(segments, segment)
	r := newRecord(recordPayload{snapshot: id, scope: scope, created: now, expires: expires, segments: segments})
	if e := validateRecord(r); e != nil {
		return newRecord(recordPayload{}), e
	}
	// Check ancestral scope/MACs before sealing even when callers are internal.
	for _, segment := range segments {
		for _, c := range segment.calls {
			if _, e := s.key.verify(c.handle, scope); e != nil {
				return newRecord(recordPayload{}), e
			}
		}
	}
	return r, nil
}

// CommitAndPublish distinguishes durability from authority. Successful final
// fsync followed by cancellation/expiry leaves an immutable bounded orphan and
// invokes ZERO callbacks. No storage I/O occurs after final sync success.
// All downstream tool/status/done frames must use the supplied original-lifetime
// context. Callback errors are preserved exactly; no retry/terminal is invented.
func (l *Lease) CommitAndPublish(ctx context.Context, publication credentials.PublicationLease, r Record, emit func(context.Context) error) (durable bool, err error) {
	if emit == nil || publication.Check() != nil || ctx.Err() != nil || l.Check() != nil {
		return false, failure(LifecycleChanged)
	}
	s := l.store
	s.mu.Lock()
	if l.checkLocked() != nil || s.key == nil {
		s.mu.Unlock()
		return false, failure(LifecycleChanged)
	}
	now, e := s.clock(ctx)
	if e != nil {
		s.mu.Unlock()
		return false, e
	}
	if e = checkExpiry(r, now); e != nil {
		s.mu.Unlock()
		return false, e
	}
	for _, segment := range r.segments {
		for _, c := range segment.calls {
			if _, e = s.key.verify(c.handle, r.scope); e != nil {
				s.mu.Unlock()
				return false, e
			}
		}
	}
	b, e := s.key.seal(r)
	if e != nil {
		s.mu.Unlock()
		return false, e
	}
	defer clear(b)
	if e = s.reserve(len(b), now); e != nil {
		s.mu.Unlock()
		return false, e
	}
	guard := func() error {
		if ctx.Err() != nil || l.original.Err() != nil || l.scope.Err() != nil || publication.Check() != nil {
			return failure(LifecycleChanged)
		}
		current, e := s.nowFloor()
		if e != nil {
			return e
		}
		return checkExpiry(r, current)
	}
	// Fault hooks may stall inside sync. Intermediate direct guards never grant
	// authority; the mandatory post-LAST-sync guards below do that separately.
	step := func(label string) error {
		if s.fault != nil {
			if e := s.fault(label); e != nil {
				return e
			}
		}
		if label != "after-dirsync" {
			return guard()
		}
		return nil
	}
	if s.dir.Atomic(ctx, recordName(r.snapshot), b, step) != nil {
		s.mu.Unlock()
		return false, failure(CommitFailed)
	}
	durable = true
	// No more FS operations, including deferred unlink, after this point.
	e = guard()
	s.mu.Unlock()
	if e != nil {
		return true, e
	}
	if l.Check() != nil {
		return true, failure(LifecycleChanged)
	}
	if e = guard(); e != nil {
		return true, e
	}
	// Done must observe manager life even if a concurrent local I/O holds
	// store.mu and delays Suspend's scope cancellation. This is bounded standard
	// cancellation linkage, not a waiter/stream worker; Err still reads ALL direct
	// sources synchronously instead of trusting AfterFunc scheduling.
	scopeContext := l.Context()
	linked, cancel := context.WithCancel(scopeContext)
	stopPublication := context.AfterFunc(publication.Context(), cancel)
	defer func() { stopPublication(); cancel() }()
	original := newEmitContext(emitContextPayload{context: linked, scope: scopeContext, publication: publication.Context(), source: ctx})
	if original.Err() != nil {
		return true, failure(LifecycleChanged)
	}
	// Cancellation linkage itself is not authority: repeat the direct-source
	// AND immutable-expiry guard after all setup, immediately before emit.
	if e = guard(); e != nil {
		return true, e
	}
	return true, emit(original)
}

type emitContextPayload struct {
	context                    context.Context
	publication, source, scope context.Context
}

func (c emitContext) Err() error {
	if c.emitContextPayload == nil || c.source == nil || c.publication == nil || c.context == nil {
		return context.Canceled
	}
	if c.scope != nil {
		if e := c.scope.Err(); e != nil {
			return e
		}
	}
	if e := c.source.Err(); e != nil {
		return e
	}
	if e := c.publication.Err(); e != nil {
		return e
	}
	return c.context.Err()
}
func checkExpiry(r Record, now int64) error {
	if r.recordPayload == nil || len(r.segments) == 0 {
		return failure(Corrupt)
	}
	if now >= r.expires {
		return failure(Expired)
	}
	for _, s := range r.segments {
		if s.segmentPayload == nil {
			return failure(Corrupt)
		}
		if now >= s.expires {
			return failure(Expired)
		}
	}
	return nil
}
