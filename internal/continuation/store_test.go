package continuation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

type fixture struct {
	disk    *credentials.DiskStore
	store   *Store
	manager *credentials.Manager
	path    string
	now     atomic.Int64
	id      credentials.Identity
	wrap    func(credentials.Storage) credentials.Storage
}

func testRecord(now time.Time) credentials.Record {
	body, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "synthetic-account"}, "exp": now.Add(time.Hour).Unix()})
	token := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." + base64.RawURLEncoding.EncodeToString(body) + ".c2ln"
	return credentials.Record{Schema: 1, Provider: "codex", Registration: credentials.ClientID, State: "active", Generation: 1, AuthEpoch: 1, Account: "synthetic-account", Access: token, Refresh: "synthetic-refresh", IssuedAt: now.Add(-time.Minute), SavedAt: now, ExpiresAt: now.Add(time.Hour)}
}
func newFixture(t *testing.T, wrap ...func(credentials.Storage) credentials.Storage) *fixture {
	t.Helper()
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	f := &fixture{path: filepath.Join(root, "private"), id: credentials.Identity{Provider: "codex", Account: "synthetic-account", Epoch: 1}}
	if len(wrap) > 0 {
		f.wrap = wrap[0]
	}
	f.now.Store(1793491200000)
	disk, e := credentials.OpenStore(f.path)
	if e != nil {
		t.Fatal(e)
	}
	f.disk = disk
	now := time.UnixMilli(f.now.Load()).UTC()
	active := testRecord(now)
	intent := active
	intent.State = "unusable"
	intent.Access = ""
	intent.Refresh = ""
	intent.Account = ""
	intent.IssuedAt = time.Time{}
	intent.ExpiresAt = time.Time{}
	if e = disk.Begin(context.Background(), intent); e != nil {
		t.Fatal(e)
	}
	if e = disk.Commit(context.Background(), active); e != nil {
		t.Fatal(e)
	}
	f.open(t)
	t.Cleanup(func() { _ = f.manager.Close() })
	return f
}
func (f *fixture) open(t *testing.T) {
	t.Helper()
	cap, e := f.disk.OpenContinuationDirectory()
	if e != nil {
		t.Fatal(e)
	}
	s, e := NewStore(cap)
	if e != nil {
		t.Fatal(e)
	}
	s.now = func() time.Time { return time.UnixMilli(f.now.Load()).UTC() }
	f.store = s
	var storage credentials.Storage = f.disk
	if f.wrap != nil {
		storage = f.wrap(storage)
	}
	m, e := credentials.NewManager(storage, credentials.ManagerOptions{Dependent: s, OAuth: credentials.OAuthClient{Now: s.now}})
	if e != nil {
		t.Fatal(e)
	}
	f.manager = m
}
func (f *fixture) lease(t *testing.T, ctx context.Context, messages []atuin.Message) (*Lease, Commitment) {
	t.Helper()
	l, e := f.store.Acquire(ctx, messages)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(l.Close)
	p, e := l.Policy(atuin.Request{}, []byte("stable instructions"), []byte("stable ordered definitions"))
	if e != nil {
		t.Fatal(e)
	}
	scope, e := l.Scope(newScopeInput(scopeInputPayload{identity: f.id, model: "synthetic-model", session: "synthetic-session", policy: p}))
	if e != nil {
		t.Fatal(e)
	}
	return l, scope
}
func (f *fixture) build(t *testing.T, l *Lease, scope Commitment, prefix []atuin.Message, ancestry []Segment) Record {
	t.Helper()
	p, e := l.Prefix(prefix)
	if e != nil {
		t.Fatal(e)
	}
	capture := NewTextCapture()
	_ = capture.Add(" \tanswer \n")
	text, recipe, e := l.FinishText(capture, []string{" \t", "answer \n"})
	if e != nil {
		t.Fatal(e)
	}
	n := len(ancestry)
	backend := fmt.Sprintf("backend-%d", n)
	items := []Item{newItem(itemPayload{kind: 3, iD: fmt.Sprintf("reasoning-%d", n), encryptedPresence: 2, encrypted: "PRIVATE-OPAQUE", summary: []Part{newPart(partPayload{kind: 2, text: "PRIVATE-SUMMARY"})}}), newItem(itemPayload{kind: 1, iD: fmt.Sprintf("message-%d", n), status: "completed", statusPresence: 2, contentPresence: 2, content: []Part{newPart(partPayload{kind: 1, span: 0}), newPart(partPayload{kind: 1, span: 1})}}), newItem(itemPayload{kind: 2, iD: fmt.Sprintf("function-%d", n), callID: backend, name: "read_file", arguments: `{"file_path":"synthetic","extra":18446744073709551615}`})}
	r, e := l.Build(scope, p, text, recipe, []Call{newCall(callPayload{backend: newBackendCallID(backendCallIDPayload{value: backend}), name: "read_file"})}, items, ancestry)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func (f *fixture) publish(l *Lease, ctx context.Context, r Record, emit func(context.Context) error) (bool, error) {
	durable := false
	e := f.manager.WithIdentityPublication(ctx, f.id, func(p credentials.PublicationLease) error {
		var e error
		durable, e = l.CommitAndPublish(ctx, p, r, emit)
		return e
	})
	return durable, e
}
func TestFinalSyncOriginalCancellationAndExpiry(t *testing.T) {
	for _, mode := range []string{"control", "cancel-original", "expire-inherited"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			prefix := []atuin.Message{textMessage("user", "PRIVATE-USER-PROMPT")}
			l, scope := f.lease(t, ctx, prefix)
			r := f.build(t, l, scope, prefix, nil)
			if mode == "expire-inherited" {
				r.expires = f.now.Load() + 1000
				r.segments[0].expires = r.expires
			}
			entered, release := make(chan struct{}), make(chan struct{})
			f.store.fault = func(step string) error {
				if step == "after-dirsync" {
					close(entered)
					<-release
				}
				return nil
			}
			type result struct {
				durable bool
				e       error
			}
			done := make(chan result, 1)
			var emits atomic.Int32
			go func() {
				d, e := f.publish(l, ctx, r, func(c context.Context) error {
					if c.Err() != nil {
						return c.Err()
					}
					emits.Add(1)
					return nil
				})
				done <- result{d, e}
			}()
			<-entered
			if mode == "cancel-original" {
				cancel()
			}
			if mode == "expire-inherited" {
				f.now.Add(1000)
			}
			close(release)
			got := <-done
			if !got.durable {
				t.Fatal("successful final fsync not durable")
			}
			if mode == "control" {
				if got.e != nil || emits.Load() != 1 {
					t.Fatal("live control failed", got.e)
				}
			} else {
				if got.e == nil || emits.Load() != 0 {
					t.Fatal("post-sync authority escaped")
				}
			}
			if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", recordName(r.snapshot))); e != nil {
				t.Fatal("bounded orphan removed")
			}
		})
	}
}
func TestDurableRestartSelfContainedAndRefreshScope(t *testing.T) {
	f := newFixture(t)
	prefix := []atuin.Message{textMessage("user", "PRIVATE-USER-PROMPT")}
	l, scope := f.lease(t, context.Background(), prefix)
	r := f.build(t, l, scope, prefix, nil)
	if d, e := f.publish(l, context.Background(), r, func(context.Context) error { return nil }); !d || e != nil {
		t.Fatal(e)
	}
	l.Close()
	messages := history(r.segments[0], prefix, "answer \n", false, false)
	// Same-epoch suspend/activate preserves files/root but revokes old leases.
	f.store.Suspend(credentials.SuspendRefresh)
	f.store.Activate(f.id)
	l2, scope2 := f.lease(t, context.Background(), messages)
	loaded, matches, e := l2.Load(messages, scope2)
	if e != nil || len(matches) != 1 {
		t.Fatal("same epoch state lost", e)
	}
	next := f.build(t, l2, scope2, messages, loaded.segments)
	if next.expires != r.expires {
		t.Fatal("ancestry TTL renewed")
	}
	if _, e = f.publish(l2, context.Background(), next, func(context.Context) error { return nil }); e != nil {
		t.Fatal(e)
	}
	l2.Close()
	if e = f.manager.Close(); e != nil {
		t.Fatal(e)
	}
	disk, e := credentials.OpenStore(f.path)
	if e != nil {
		t.Fatal(e)
	}
	f.disk = disk
	f.open(t)
	// New record proves old contract without old file.
	if e = os.Remove(filepath.Join(f.path, "continuation-v1", recordName(r.snapshot))); e != nil {
		t.Fatal(e)
	}
	messages = history(next.segments[1], messages, "answer \n", true, false)
	l3, scope3 := f.lease(t, context.Background(), messages)
	replayed, matches, e := l3.Load(messages, scope3)
	if e != nil || len(matches) != 2 {
		t.Fatal("restart self-contained ancestry failed", e)
	}
	replayed.Clear()
	l3.Close()
	raw, e := os.ReadFile(filepath.Join(f.path, "continuation-v1", recordName(next.snapshot)))
	if e != nil {
		t.Fatal(e)
	}
	for _, marker := range []string{"PRIVATE-USER-PROMPT", "PRIVATE-SUMMARY", "PRIVATE-OPAQUE", "backend-0", "stable instructions", "stable ordered definitions"} {
		if strings.Contains(string(raw), marker) {
			t.Fatal("private marker persisted in clear")
		}
	}
}
func TestFaultsLeaveNoAuthorityAndBoundedPending(t *testing.T) {
	for _, stage := range []string{"create", "write", "fsync", "close", "rename", "dirsync", "after-dirsync"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixture(t)
			prefix := []atuin.Message{textMessage("user", "x")}
			l, scope := f.lease(t, context.Background(), prefix)
			r := f.build(t, l, scope, prefix, nil)
			f.store.fault = func(step string) error {
				if step == stage {
					return fmt.Errorf("PRIVATE-PATH-RAW-CAUSE")
				}
				return nil
			}
			called := false
			if _, e := f.publish(l, context.Background(), r, func(context.Context) error { called = true; return nil }); e == nil || called || strings.Contains(fmt.Sprint(e), "PRIVATE") {
				t.Fatal("failed commit granted authority")
			}
			entries, e := f.store.dir.Scan()
			if e != nil || len(entries) > 3 {
				t.Fatal("unbounded crash residue")
			}
			l.Close()
			if e = f.manager.Close(); e != nil {
				t.Fatal(e)
			}
			disk, e := credentials.OpenStore(f.path)
			if e != nil {
				t.Fatal(e)
			}
			f.disk = disk
			f.open(t)
			entries, e = f.store.dir.Scan()
			if e != nil {
				t.Fatal(e)
			}
			for _, v := range entries {
				if strings.HasPrefix(v.Name(), ".pending-") {
					t.Fatal("owned pending not cleaned")
				}
			}
		})
	}
}
func TestClockHighwaterRollbackAndMissingKey(t *testing.T) {
	f := newFixture(t)
	prefix := []atuin.Message{textMessage("user", "x")}
	l, scope := f.lease(t, context.Background(), prefix)
	r := f.build(t, l, scope, prefix, nil)
	if _, e := f.publish(l, context.Background(), r, func(context.Context) error { return nil }); e != nil {
		t.Fatal(e)
	}
	l.Close()
	messages := history(r.segments[0], prefix, "answer \n", true, false)
	f.now.Add(-60001)
	l2, e := f.store.Acquire(context.Background(), messages)
	if e != nil {
		t.Fatal(e)
	}
	defer l2.Close()
	if _, _, e = l2.Load(messages, scope); e == nil {
		t.Fatal("rollback accepted")
	}
	f.now.Add(60001)
	f.store.Suspend(credentials.SuspendRefresh)
	f.store.Activate(f.id)
	if e = os.Remove(filepath.Join(f.path, "continuation-v1", "key.bundle")); e != nil {
		t.Fatal(e)
	}
	l3, e := f.store.Acquire(context.Background(), messages)
	if e != nil {
		t.Fatal(e)
	}
	defer l3.Close()
	if _, _, e = l3.Load(messages, scope); e == nil {
		t.Fatal("missing key accepted")
	}
	if _, e = os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); !os.IsNotExist(e) {
		t.Fatal("missing marked key silently initialized")
	}
}
func TestFourLeasesSuspendJoinsOnlyNativeCompletion(t *testing.T) {
	f := newFixture(t)
	leases := make([]*Lease, 4)
	for j := range leases {
		l, e := f.store.Acquire(context.Background(), nil)
		if e != nil {
			t.Fatal(e)
		}
		leases[j] = l
		defer l.Close()
	}
	if _, e := f.store.Acquire(context.Background(), nil); e == nil {
		t.Fatal("fifth lease admitted")
	}
	if e := leases[0].MarkStreamStarted(); e != nil {
		t.Fatal(e)
	}
	suspended := make(chan struct{})
	go func() { f.store.Suspend(credentials.SuspendLogout); close(suspended) }()
	<-leases[0].Context().Done()
	select {
	case <-suspended:
		t.Fatal("Suspend returned before native cleanup")
	default:
	}
	leases[0].StreamDone()
	select {
	case <-suspended:
	case <-time.After(time.Second):
		t.Fatal("Suspend waited for Publish/Close instead of stream completion")
	}
	if e := leases[1].MarkStreamStarted(); e == nil {
		t.Fatal("revoked unstarted lease opened")
	}
	for _, l := range leases {
		if l.Context().Err() == nil || l.Check() == nil {
			t.Fatal("revoked lease live")
		}
	}
}
func TestKeyCorruptionPurgeAndLogoutCommit(t *testing.T) {
	f := newFixture(t)
	l, _ := f.lease(t, context.Background(), nil)
	l.Close()
	key := filepath.Join(f.path, "continuation-v1", "key.bundle")
	if e := os.WriteFile(key, []byte("damaged key contents"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := f.manager.Logout(context.Background()); e != nil {
		t.Fatal("owned corrupted key blocked authorized purge", e)
	}
	if _, e := os.Stat(key); !os.IsNotExist(e) {
		t.Fatal("key retained after logout")
	}
	r, e := f.disk.Load()
	if e != nil || r.State != "logged_out" {
		t.Fatal("logout credentials not committed")
	}
}
