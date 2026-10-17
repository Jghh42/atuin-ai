// Package catalog owns immutable, credential-bound bundled model snapshots.
// Remote metadata is used only by explicit diagnostics, never for availability.
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
	ErrUnavailable  = errors.New("bundled model catalog unavailable; valid application login required")
	ErrUnauthorized = errors.New("model catalog requires reauthentication")
	ErrTransient    = errors.New("temporary model discovery failure")
	ErrInvalid      = errors.New("invalid or incompatible model catalog")
	ErrOverride     = errors.New("default_model is not a known bundled ID; remove or update the override")
)

// FetchTimeout bounds explicit diagnostic metadata requests only.
const FetchTimeout = 8 * time.Second

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

// Fetcher is used by explicit diagnostic/proof tooling, never by Cache.
type Fetcher interface {
	Fetch(context.Context, credentials.Credential) ([]Entry, error)
}

type Snapshot struct {
	response Response
	identity credentials.Identity
}

// Identity binds inference to the account/epoch checked before listing.
func (s Snapshot) Identity() credentials.Identity { return s.identity }

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

// Build preserves historical remote replay-proof selection for diagnostics and
// proof tooling ONLY. Runtime availability uses BuildBundled instead.
func Build(entries []Entry, override string) (Snapshot, error) {
	return selectSnapshot(entries, override, replayProven)
}

func replayProven(id string) bool { return id == "gpt-6.1-sol" }

// Eligibility contains only sanitized counters/booleans, never model metadata.
type Eligibility struct {
	ListedCount, EligibleCount  int
	ProvenPresent, ProvenListed bool
}

// InspectEligibility examines historical replay proof, NOT runtime membership.
// Luna's bundled availability does not assert historical replay or live proof.
func InspectEligibility(entries []Entry) Eligibility {
	var result Eligibility
	for _, m := range entries {
		if m.Visibility == "list" {
			result.ListedCount++
		}
		if replayProven(m.ID) {
			result.ProvenPresent = true
			if m.Visibility == "list" {
				result.ProvenListed = true
				result.EligibleCount++
			}
		}
	}
	return result
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
	return Snapshot{response: r}, nil
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

// BuildBundled constructs a fresh snapshot of the exact reviewed native IDs.
// These are known metadata, not an account entitlement or capacity promise.
func BuildBundled(override string) (Snapshot, error) {
	const sol = "gpt-6.1-sol"
	const luna = "gpt-6-luna"
	if override != "" && override != sol && override != luna {
		return Snapshot{}, ErrOverride
	}
	def := sol
	if override != "" {
		def = override
	}
	return Snapshot{response: Response{Default: def, Models: []Model{
		{Alias: sol, Name: "Sol", Description: "Native Codex Sol"},
		{Alias: luna, Name: "Luna", Description: "Native Codex Luna (live validation pending)"},
	}}}, nil
}

// Cache pins local immutable metadata to a credential identity. There is no
// remote fetch, TTL, background worker, or stale credential fallback.
type Cache struct {
	mu        sync.Mutex
	source    credentials.Source
	override  string
	snapshot  Snapshot
	lifecycle uint64
}

func New(source credentials.Source, override string) *Cache {
	return &Cache{source: source, override: override}
}

// Invalidate must be called synchronously by lifecycle writers after publishing
// new credential state. Unlocked obsolete lookups cannot repopulate the cache.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lifecycle++
	c.snapshot = Snapshot{}
}

func (c *Cache) Get(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if c.source == nil {
		return Snapshot{}, ErrUnavailable
	}
	c.mu.Lock()
	revision := c.lifecycle
	c.mu.Unlock()
	// Preflight on EVERY lookup, including cached metadata. Current may refresh
	// application credentials, but never requests remote model metadata.
	cred, err := c.source.Current(ctx)
	if ctx.Err() != nil {
		return Snapshot{}, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if revision != c.lifecycle {
		return Snapshot{}, ErrUnavailable
	}
	if err != nil || cred.Identity.Provider != "codex" || cred.Identity.Account == "" || cred.AccessToken == "" {
		c.lifecycle++
		c.snapshot = Snapshot{}
		return Snapshot{}, ErrUnauthorized
	}
	if c.snapshot.response.Default == "" || c.snapshot.identity != cred.Identity {
		s, err := BuildBundled(c.override)
		if err != nil {
			return Snapshot{}, err
		}
		s.identity = cred.Identity
		c.snapshot = s
	}
	return c.snapshot, nil
}
