//go:build linux || darwin

package continuation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

// Store owns one fixed-child capability. No whole-store plaintext cache or
// background maintenance workers exist. The credential flock must outlive it.
type storePayload struct {
	mu        *sync.Mutex
	dir       *credentials.ContinuationDirectory
	key       *keys
	persisted int64
	floor     *atomic.Int64
	now       func() time.Time
	active    bool
	identity  credentials.Identity
	scope     context.Context
	cancel    context.CancelFunc
	leases    map[*Lease]bool
	closed    bool
	closeErr  error // immutable fixed result after directory close
	// fault runs synchronously, including after the FINAL dirsync test barrier.
	fault func(string) error
}

func NewStore(dir *credentials.ContinuationDirectory) (*Store, error) {
	if dir == nil {
		return nil, failure(Unavailable)
	}
	s := ownedPointer(newStore(storePayload{dir: dir, now: time.Now, leases: map[*Lease]bool{}}))
	entries, e := s.scan()
	if e != nil {
		return nil, e
	}
	changed := false
	for _, v := range entries {
		if strings.HasPrefix(v.Name(), ".pending-") {
			if dir.Remove(v.Name()) != nil {
				return nil, failure(Unavailable)
			}
			changed = true
		}
	}
	if changed && dir.Sync() != nil {
		return nil, failure(Unavailable)
	}
	return s, nil
}
func (s *Store) scan() ([]credentials.ContinuationEntry, error) {
	entries, e := s.dir.Scan()
	if e != nil {
		return nil, failure(Unavailable)
	}
	var total int64
	records := 0
	for _, v := range entries {
		if v.Bytes() < 0 || v.Bytes() > MaxStoreBytes-total {
			return nil, failure(Limit)
		}
		total += v.Bytes()
		if strings.HasSuffix(v.Name(), ".sealed") {
			records++
		}
		// Content faults are selected-read failures, not a reason to disable
		// legacy credentials or prevent attended, safely-owned logout purge.
		// Reads still reject oversized key/records BEFORE allocating content.
	}
	if records > MaxRecords || total > MaxStoreBytes {
		return nil, failure(Limit)
	}
	return entries, nil
}

// Suspend is unconditional, in-memory only and joins started stream cleanup,
// NOT transaction Close or pending Publish (which may await the manager gate).
func (s *Store) Suspend(_ credentials.SuspendReason) {
	s.mu.Lock()
	s.active = false
	if s.cancel != nil {
		s.cancel()
	}
	if s.key != nil {
		clear(s.key.root[:])
		s.key = nil
	}
	s.persisted = 0
	waits := make([]<-chan struct{}, 0, MaxScopes)
	for l := range s.leases {
		l.revoked = true
		l.cancel()
		if l.started && !l.completed {
			waits = append(waits, l.done)
		}
	}
	s.mu.Unlock()
	for _, done := range waits {
		<-done
	}
}
func (s *Store) Activate(id credentials.Identity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.active && s.identity == id {
		return
	}
	// Lifecycle callers suspend/join before activation. Never rebind live leases.
	for l := range s.leases {
		if !l.revoked {
			return
		}
	}
	s.identity = id
	s.scope, s.cancel = context.WithCancel(context.Background())
	s.active = true
}
func (s *Store) Purge(intent credentials.DurableIntent) error {
	if !intent.Valid() {
		return failure(LifecycleChanged)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.active {
		return failure(LifecycleChanged)
	}
	entries, e := s.dir.Scan()
	if e != nil {
		return failure(Unavailable)
	}
	// Never parse a damaged key before an explicitly authorized owned purge.
	if s.dir.Remove("key.bundle") != nil || s.dir.Sync() != nil {
		return failure(Unavailable)
	}
	for _, v := range entries {
		if v.Name() != "key.bundle" && s.dir.Remove(v.Name()) != nil {
			return failure(Unavailable)
		}
	}
	if s.dir.Sync() != nil {
		return failure(Unavailable)
	}
	s.floor.Store(0)
	s.persisted = 0
	return nil
}
func (s *Store) Close() error {
	s.Suspend(credentials.SuspendShutdown)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	if s.dir.Close() != nil {
		s.closeErr = failure(Unavailable)
	}
	return s.closeErr
}
func (s *Store) nowFloor() (int64, error) {
	t := s.now()
	if t.Year() < 2020 || t.Year() > 9999 {
		return 0, failure(ClockInvalid)
	}
	n := t.UnixMilli()
	floor := s.floor.Load()
	if n < floor-60000 {
		return 0, failure(ClockInvalid)
	}
	if n < floor {
		return floor, nil
	}
	return s.raiseFloor(n), nil
}
func (s *Store) raiseFloor(n int64) int64 {
	for {
		old := s.floor.Load()
		if n <= old {
			return old
		}
		if s.floor.CompareAndSwap(old, n) {
			return n
		}
	}
}
func (s *Store) clock(ctx context.Context) (int64, error) {
	n, e := s.nowFloor()
	if e != nil {
		return 0, e
	}
	if n > s.persisted {
		entries, e := s.scan()
		if e != nil {
			return 0, e
		}
		var bytes int64
		for _, v := range entries {
			bytes += v.Bytes()
		}
		if bytes+120 > MaxStoreBytes || len(entries)+1 > MaxEntries {
			if e := s.reserve(120, n); e != nil {
				return 0, e
			}
		}
		b := bundle(s.key, s.key.owner(s.identity.Provider, s.identity.Account, s.identity.Epoch), n)
		defer clear(b)
		if s.dir.Atomic(ctx, "key.bundle", b, s.fault) != nil {
			return 0, failure(CommitFailed)
		}
		s.persisted = n
		s.raiseFloor(n)
	}
	return n, nil
}
func (s *Store) ensureKey(ctx context.Context, marked bool) error {
	if s.key != nil {
		return nil
	}
	b, exists, e := s.dir.Read("key.bundle", 4096)
	if e != nil {
		return failure(Unavailable)
	}
	defer clear(b)
	if exists {
		k, owner, floor, e := decodeBundle(b)
		if e != nil {
			return e
		}
		expected := k.owner(s.identity.Provider, s.identity.Account, s.identity.Epoch)
		if !equal(owner, expected) {
			clear(k.root[:])
			return failure(Unavailable)
		}
		s.key = k
		s.persisted = floor
		s.raiseFloor(floor)
		return nil
	}
	entries, e := s.scan()
	if e != nil {
		return e
	}
	if marked || len(entries) != 0 {
		return failure(Missing)
	}
	k := ownedPointer(newKeys(keysPayload{}))
	if _, e := rand.Read(k.root[:]); e != nil {
		return failure(Unavailable)
	}
	if _, e := rand.Read(k.id[:]); e != nil {
		clear(k.root[:])
		return failure(Unavailable)
	}
	s.key = k
	if _, e := s.clock(ctx); e != nil {
		clear(k.root[:])
		s.key = nil
		return e
	}
	return nil
}
func recordName(id SnapshotID) string { return hex.EncodeToString(id.bytes[:]) + ".sealed" }
func (s *Store) load(id SnapshotID) (Record, error) {
	b, exists, e := s.dir.Read(recordName(id), MaxRecordBytes)
	if e != nil {
		return newRecord(recordPayload{}), failure(Corrupt)
	}
	if !exists {
		return newRecord(recordPayload{}), failure(Missing)
	}
	defer clear(b)
	r, e := s.key.open(b)
	if e != nil {
		return newRecord(recordPayload{}), e
	}
	if !sameSnapshot(r.snapshot, id) {
		r.Clear()
		return newRecord(recordPayload{}), failure(Corrupt)
	}
	return r, nil
}

// reserve authenticates at most ONE bounded plaintext record at a time. Pending
// crash residue and the temporary sealed bytes count against actual disk quota.
func (s *Store) reserve(n int, now int64) error {
	entries, e := s.scan()
	if e != nil {
		return e
	}
	// No whole-store decrypt on ordinary commits. Authenticate/prune only when
	// capacity requires maintenance; authorization expiry is checked on lookup.
	var occupied int64
	count := 0
	pending := false
	for _, v := range entries {
		occupied += v.Bytes()
		if strings.HasSuffix(v.Name(), ".sealed") {
			count++
		}
		if strings.HasPrefix(v.Name(), ".pending-") {
			pending = true
		}
	}
	if !pending && count < MaxRecords && occupied+int64(n) <= MaxStoreBytes && len(entries)+1 <= MaxEntries {
		return nil
	}
	type victim struct {
		name                   string
		created, expires, size int64
	}
	victims := make([]victim, 0, MaxRecords)
	var total int64
	records := 0
	for _, v := range entries {
		total += v.Bytes()
		if strings.HasSuffix(v.Name(), ".sealed") {
			records++
			b, ok, e := s.dir.Read(v.Name(), MaxRecordBytes)
			if e != nil || !ok {
				return failure(Corrupt)
			}
			r, e := s.key.open(b)
			clear(b)
			if e != nil {
				return e
			}
			victims = append(victims, victim{v.Name(), r.created, r.expires, v.Bytes()})
			r.Clear()
		}
	}
	sort.Slice(victims, func(i, j int) bool {
		a, b := victims[i], victims[j]
		ae, be := now >= a.expires, now >= b.expires
		if ae != be {
			return ae
		}
		if a.created != b.created {
			return a.created < b.created
		}
		return a.name < b.name
	})
	changed := false
	remaining := len(entries)
	for _, v := range victims {
		if now < v.expires && records < MaxRecords && total+int64(n) <= MaxStoreBytes && remaining+1 <= MaxEntries {
			break
		}
		if s.dir.Remove(v.name) != nil {
			return failure(CommitFailed)
		}
		changed = true
		records--
		remaining--
		total -= v.size
	}
	// Safely-owned pending files can be removed, but were NOT ignored in scan.
	for _, v := range entries {
		if strings.HasPrefix(v.Name(), ".pending-") {
			if s.dir.Remove(v.Name()) != nil {
				return failure(CommitFailed)
			}
			changed = true
			remaining--
			total -= v.Bytes()
		}
	}
	if changed && s.dir.Sync() != nil {
		return failure(CommitFailed)
	}
	if records >= MaxRecords || total+int64(n) > MaxStoreBytes || remaining+1 > MaxEntries {
		return failure(Limit)
	}
	return nil
}

// Policy/Prefix are available only on an admitted active lease; no credential
// token, volatile turn context or raw public transcript is persisted.
func (l *Lease) Policy(r atuin.Request, rendered, defs []byte) (Commitment, error) {
	s := l.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.checkLocked() != nil {
		return newCommitment(commitmentPayload{}), failure(LifecycleChanged)
	}
	if e := s.ensureKey(l.original, l.marked); e != nil {
		return newCommitment(commitmentPayload{}), e
	}
	if _, e := s.clock(l.original); e != nil {
		return newCommitment(commitmentPayload{}), e
	}
	return s.key.policy(r, rendered, defs)
}
func (l *Lease) Scope(in ScopeInput) (Commitment, error) {
	s := l.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.checkLocked() != nil || in.scopeInputPayload == nil || in.identity != s.identity {
		return newCommitment(commitmentPayload{}), failure(LifecycleChanged)
	}
	if in.model == "" || in.session == "" || len(in.model) > 256 || len(in.session) > 256 {
		return newCommitment(commitmentPayload{}), failure(BindingMismatch)
	}
	if e := s.ensureKey(l.original, l.marked); e != nil {
		return newCommitment(commitmentPayload{}), e
	}
	if _, e := s.clock(l.original); e != nil {
		return newCommitment(commitmentPayload{}), e
	}
	return s.key.scope(in), nil
}
func (l *Lease) Prefix(messages []atuin.Message) (Prefix, error) {
	s := l.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.checkLocked() != nil || s.key == nil {
		return newPrefix(prefixPayload{}), failure(LifecycleChanged)
	}
	return s.key.prefix(messages)
}
func (l *Lease) FinishText(c *TextCapture, parts []string) (TextExpectation, TextRecipe, error) {
	s := l.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.checkLocked() != nil || s.key == nil {
		return newTextExpectation(textExpectationPayload{}), newTextRecipe(textRecipePayload{}), failure(LifecycleChanged)
	}
	return c.Finish(s.key, parts)
}
func (l *Lease) Load(messages []atuin.Message, scope Commitment) (Record, []Match, error) {
	id, marked, e := latest(messages)
	if e != nil {
		return newRecord(recordPayload{}), nil, e
	}
	if !marked {
		return newRecord(recordPayload{}), nil, nil
	}
	s := l.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.checkLocked() != nil {
		return newRecord(recordPayload{}), nil, failure(LifecycleChanged)
	}
	if e := s.ensureKey(l.original, true); e != nil {
		return newRecord(recordPayload{}), nil, e
	}
	now, e := s.clock(l.original)
	if e != nil {
		return newRecord(recordPayload{}), nil, e
	}
	r, e := s.load(id)
	if e != nil {
		return newRecord(recordPayload{}), nil, e
	}
	if now >= r.expires {
		r.Clear()
		return newRecord(recordPayload{}), nil, failure(Expired)
	}
	matches, e := s.key.verifyRecord(r, messages, scope)
	if e != nil {
		r.Clear()
		return newRecord(recordPayload{}), nil, e
	}
	current, e := s.nowFloor()
	if e == nil {
		e = checkExpiry(r, current)
	}
	if e == nil {
		e = l.checkLocked()
	}
	if e != nil {
		r.Clear()
		return newRecord(recordPayload{}), nil, e
	}
	return r, matches, nil
}
