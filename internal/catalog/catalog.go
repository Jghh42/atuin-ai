// Package catalog owns immutable, account-scoped discovery snapshots.
package catalog

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

var (
	ErrUnavailable  = errors.New("model discovery unavailable; application login and successful discovery required")
	ErrUnauthorized = errors.New("model discovery requires reauthentication")
	ErrTransient    = errors.New("temporary model discovery failure")
	ErrInvalid      = errors.New("invalid or incompatible model catalog")
	ErrOverride     = errors.New("default_model is not an eligible discovered ID; remove or update the override")
)

const (
	FreshFor     = 15 * time.Minute
	StaleFor     = time.Hour
	FetchTimeout = 8 * time.Second
)

type Model struct {
	Alias       string `json:"alias"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Response struct {
	Default string  `json:"default"`
	Models  []Model `json:"models"`
}

// Entry is the bounded provider-neutral subset of native catalog metadata.
// No release timestamp or model-version convention is verified in the pinned
// source: priority, not a guessed numeric slug ordering, orders releases.
type Entry struct {
	ID, Name, Description, Visibility string
	Priority                          int32
}
type Fetcher interface {
	Fetch(context.Context, credentials.Credential) ([]Entry, error)
}
type Snapshot struct{ response Response }

func (s Snapshot) Response() Response {
	r := s.response
	r.Models = append([]Model(nil), r.Models...)
	return r
}
func (s Snapshot) Resolve(id string) string {
	for _, m := range s.response.Models {
		if id == m.Alias {
			return id
		}
	}
	return s.response.Default
}

// Build validates a bounded immutable snapshot using the internal proof allowlist.
func Build(entries []Entry, override string) (Snapshot, error) {
	return selectSnapshot(entries, override, func(id string) bool { return id == "gpt-6.1-sol" })
}

// Validate checks metadata independently of replay eligibility/default selection.
func Validate(entries []Entry) error {
	if len(entries) == 0 || len(entries) > 256 {
		return ErrInvalid
	}
	seen := make(map[string]bool, len(entries))
	for _, m := range entries {
		if !validID(m.ID) || len(m.Name) == 0 || len(m.Name) > 256 || len(m.Description) > 4096 || !utf8.ValidString(m.Name) || !utf8.ValidString(m.Description) || seen[m.ID] || (m.Visibility != "list" && m.Visibility != "hide" && m.Visibility != "none") {
			return ErrInvalid
		}
		seen[m.ID] = true
	}
	return nil
}

// selectSnapshot uses an internal proof allowlist, never a user-supplied list.
func selectSnapshot(entries []Entry, override string, eligible func(string) bool) (Snapshot, error) {
	if err := Validate(entries); err != nil {
		return Snapshot{}, err
	}
	list := make([]Entry, 0, len(entries))
	for _, m := range entries {
		if m.Visibility == "list" && eligible(m.ID) {
			list = append(list, m)
		}
	}
	if len(list) == 0 {
		return Snapshot{}, ErrInvalid
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority != list[j].Priority {
			return list[i].Priority < list[j].Priority
		}
		return list[i].ID < list[j].ID
	})
	def := list[0].ID
	for _, m := range list {
		if strings.HasSuffix(m.ID, "-sol") {
			def = m.ID
			break
		}
	}
	if override != "" {
		if !contains(list, override) {
			return Snapshot{}, ErrOverride
		}
		def = override
	}
	r := Response{Default: def, Models: make([]Model, len(list))}
	for i, m := range list {
		r.Models[i] = Model{m.ID, m.Name, m.Description}
	}
	return Snapshot{r}, nil
}
func contains(es []Entry, id string) bool {
	for _, e := range es {
		if e.ID == id {
			return true
		}
	}
	return false
}
func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

type flight struct {
	done      chan struct{}
	cancel    context.CancelFunc
	waiters   int
	snapshot  Snapshot
	err       error
	revision  uint64
	validated time.Time
}
type Cache struct {
	mu        sync.Mutex
	source    credentials.Source
	fetcher   Fetcher
	now       func() time.Time
	override  string
	identity  credentials.Identity
	snapshot  Snapshot
	validated time.Time
	active    *flight
	revision  uint64
	lifecycle uint64
}

func New(source credentials.Source, fetcher Fetcher, override string, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{source: source, fetcher: fetcher, override: override, now: now}
}

// Invalidate must be called synchronously by lifecycle writers. It also prevents
// in-flight discovery from repopulating a logged-out or changed account cache.
func (c *Cache) Invalidate() { c.mu.Lock(); defer c.mu.Unlock(); c.lifecycle++; c.clearLocked() }
func (c *Cache) clearLocked() {
	c.revision++
	if c.active != nil {
		c.active.cancel()
		c.active = nil
	}
	c.snapshot = Snapshot{}
	c.validated = time.Time{}
	c.identity = credentials.Identity{}
}
func (c *Cache) Get(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if c.source == nil || c.fetcher == nil {
		return Snapshot{}, ErrUnavailable
	}
	c.mu.Lock()
	sourceRevision := c.lifecycle
	c.mu.Unlock()
	cred, err := c.source.Current(ctx)
	if ctx.Err() != nil {
		return Snapshot{}, ctx.Err()
	}
	c.mu.Lock()
	if c.lifecycle != sourceRevision {
		c.mu.Unlock()
		return Snapshot{}, ErrUnavailable
	}
	if err != nil || cred.Identity.Provider != "codex" || cred.Identity.Account == "" || cred.AccessToken == "" {
		c.lifecycle++
		c.clearLocked()
		c.mu.Unlock()
		return Snapshot{}, ErrUnauthorized
	}
	if c.identity != cred.Identity {
		c.clearLocked()
		c.identity = cred.Identity
	}
	age := c.now().Sub(c.validated)
	if c.snapshot.response.Default != "" && age >= 0 && age < FreshFor {
		s := c.snapshot
		c.mu.Unlock()
		return s, nil
	}
	f := c.active
	if f == nil {
		fetchCtx, cancel := context.WithTimeout(context.Background(), FetchTimeout)
		f = &flight{done: make(chan struct{}), cancel: cancel, revision: c.revision}
		c.active = f
		go c.refresh(fetchCtx, cred, f)
	}
	f.waiters++
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		c.mu.Lock()
		f.waiters--
		if f.waiters == 0 && c.active == f {
			f.cancel()
			c.active = nil
		}
		c.mu.Unlock()
		return Snapshot{}, ctx.Err()
	case <-f.done:
		return c.flightResult(cred.Identity, f)
	}
}

// flightResult is the single result-consumption boundary, including waiters
// delayed after completion while a newer refresh invalidates old snapshots.
func (c *Cache) flightResult(identity credentials.Identity, f *flight) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.identity != identity || c.revision != f.revision {
		return Snapshot{}, ErrUnauthorized
	}
	if f.snapshot.response.Default != "" {
		age := c.now().Sub(f.validated)
		if age < 0 || age > StaleFor {
			return Snapshot{}, ErrUnavailable
		}
	}
	return f.snapshot, f.err
}
func (c *Cache) refresh(ctx context.Context, cred credentials.Credential, f *flight) {
	defer f.cancel()
	entries, err := c.fetcher.Fetch(ctx, cred)
	var s Snapshot
	if err == nil {
		s, err = Build(entries, c.override)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	defer close(f.done)
	if c.active != f || c.identity != cred.Identity {
		f.err = ErrUnavailable
		return
	}
	c.active = nil
	if err == nil {
		c.snapshot = s
		c.validated = c.now()
		f.snapshot = s
		f.validated = c.validated
		return
	}
	age := c.now().Sub(c.validated)
	if errors.Is(err, ErrTransient) && c.snapshot.response.Default != "" && age >= 0 && age <= StaleFor {
		f.snapshot = c.snapshot
		f.validated = c.validated
		return
	}
	// Invalidate even already-completed stale flights. The failing flight owns
	// this new revision so its own waiters still receive the specific safe error.
	c.revision++
	f.revision = c.revision
	c.snapshot = Snapshot{}
	c.validated = time.Time{}
	f.err = err
}
