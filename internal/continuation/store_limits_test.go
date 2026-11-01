package continuation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func TestRecordCountQuotaAuthenticatedOldestEviction(t *testing.T) {
	f := newFixture(t)
	l, _ := f.lease(t, context.Background(), nil)
	defer l.Close()
	dir := filepath.Join(f.path, "continuation-v1")
	for n := 1; n <= MaxRecords; n++ {
		r := sampleRecord(t, f.store.key, 1, "", []atuin.Message{textMessage("user", "x")}, byte(n))
		b, e := f.store.key.seal(r)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, recordName(r.snapshot)), b, 0600); e != nil {
			t.Fatal(e)
		}
		clear(b)
	}
	entries, e := f.store.scan()
	if e != nil || len(entries) != 129 {
		t.Fatal("128-record control", e)
	}
	f.store.mu.Lock()
	e = f.store.reserve(1, f.now.Load())
	f.store.mu.Unlock()
	if e != nil {
		t.Fatal(e)
	}
	entries, e = f.store.scan()
	if e != nil || len(entries) != 128 {
		t.Fatal("reservation did not evict exactly oldest")
	}
	id := newSnapshotID(snapshotIDPayload{})
	id.bytes[0] = 1
	if _, e = os.Stat(filepath.Join(dir, recordName(id))); !os.IsNotExist(e) {
		t.Fatal("authenticated creation/snapshot tie-break changed")
	}
}
func TestActualByteQuotaIncludesKeyAndPendingWithoutWholeStoreRead(t *testing.T) {
	f := newFixture(t)
	l, _ := f.lease(t, context.Background(), nil)
	defer l.Close()
	path := filepath.Join(f.path, "continuation-v1", ".pending-00000000000000000000000000000000")
	file, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if e = file.Truncate(MaxStoreBytes - 120); e != nil {
		t.Fatal(e)
	}
	if e = file.Close(); e != nil {
		t.Fatal(e)
	}
	entries, e := f.store.scan()
	if e != nil {
		t.Fatal(e)
	}
	var total int64
	for _, entry := range entries {
		total += entry.Bytes()
	}
	if total != MaxStoreBytes {
		t.Fatal("actual logical bytes/key/pending not accounted")
	}
	f.store.mu.Lock()
	e = f.store.reserve(1, f.now.Load())
	f.store.mu.Unlock()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("owned crash residue not reclaimed under quota reservation")
	}
	file, e = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if e = file.Truncate(MaxStoreBytes); e != nil {
		t.Fatal(e)
	}
	file.Close()
	if _, e = f.store.scan(); e == nil {
		t.Fatal("key bytes excluded from hard total ceiling")
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("over-quota scan deleted instead of failing safely")
	}
}
func TestProtectedRecordPreAllocationAndEnvelopeCeilings(t *testing.T) {
	f := newFixture(t)
	l, scope := f.lease(t, context.Background(), nil)
	defer l.Close()
	p, _ := l.Prefix(nil)
	capture := NewTextCapture()
	expected, recipe, e := l.FinishText(capture, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = l.Build(scope, p, expected, recipe, nil, nil, nil); e == nil {
		t.Fatal("zero-call protected record")
	}
	items := makeItem(129)
	calls := []Call{newCall(callPayload{backend: NewBackendCallID("b"), name: "read_file"})}
	if _, e = l.Build(scope, p, expected, recipe, calls, items, nil); e == nil {
		t.Fatal("current output collection overflow")
	}
	r := sampleRecord(t, f.store.key, 1, "", nil, 1)
	r.segments[0].items[0].encrypted = strings.Repeat("x", MaxRecordBytes)
	if _, e = f.store.key.seal(r); e == nil {
		t.Fatal("sealed envelope cap excludes metadata")
	}
	r = sampleRecord(t, f.store.key, 1, "", nil, 1)
	r.segments[0].items[0].encrypted = strings.Repeat("x", MaxRecordBytes-2048)
	b, e := f.store.key.seal(r)
	if e != nil || len(b) > MaxRecordBytes {
		t.Fatal("bounded near-cap control", e)
	}
	if _, e = f.store.key.open(b); e != nil {
		t.Fatal("near-cap record not readable", e)
	}
	clear(b)
}
func TestPersistedClockFloorSurvivesRestart(t *testing.T) {
	f := newFixture(t)
	l, _ := f.lease(t, context.Background(), nil)
	f.now.Add(20000)
	if _, e := l.Policy(atuin.Request{}, nil, nil); e != nil {
		t.Fatal(e)
	}
	l.Close()
	if e := f.manager.Close(); e != nil {
		t.Fatal(e)
	}
	f.now.Add(-60001)
	disk, e := credentials.OpenStore(f.path)
	if e != nil {
		t.Fatal(e)
	}
	f.disk = disk
	f.open(t)
	lease, e := f.store.Acquire(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer lease.Close()
	if _, e = lease.Policy(atuin.Request{}, nil, nil); e == nil {
		t.Fatal("persisted high-water rollback lost on restart")
	}
}

type failBeginStorage struct{ credentials.Storage }

func (failBeginStorage) Begin(context.Context, credentials.Record) error { return credentials.ErrStore }
func TestActualScopeLeasesFailedBeginSuspendBeforeLifecycleReturns(t *testing.T) {
	for _, operation := range []string{"login", "logout", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			f := newFixture(t, func(s credentials.Storage) credentials.Storage { return failBeginStorage{s} })
			stream, _ := f.lease(t, context.Background(), nil)
			pending, _ := f.lease(t, context.Background(), nil)
			if e := stream.MarkStreamStarted(); e != nil {
				t.Fatal(e)
			}
			cleanup := make(chan struct{})
			callbackCanceled := make(chan struct{})
			go func() { <-stream.Context().Done(); close(callbackCanceled); <-cleanup; stream.StreamDone() }()
			result := make(chan error, 1)
			go func() {
				switch operation {
				case "login":
					result <- f.manager.Login(context.Background(), func(credentials.DevicePrompt) error { t.Error("Device flow after failed Begin"); return nil })
				case "logout":
					result <- f.manager.Logout(context.Background())
				case "refresh":
					f.now.Add(int64(time.Hour / time.Millisecond))
					_, e := f.manager.Current(context.Background())
					result <- e
				}
			}()
			<-callbackCanceled
			select {
			case <-result:
				t.Fatal("failed Begin returned before native/sink cleanup joined")
			default:
			}
			waiter := make(chan error, 1)
			go func() {
				waiter <- f.manager.WithIdentityPublication(pending.Context(), f.id, func(credentials.PublicationLease) error { t.Error("revoked pending transaction published"); return nil })
			}()
			select {
			case e := <-waiter:
				if e == nil {
					t.Fatal("revoked publication authorized")
				}
			case <-time.After(time.Second):
				t.Fatal("pending Publish deadlocked under lifecycle gate")
			}
			close(cleanup)
			select {
			case e := <-result:
				if e != credentials.ErrStore {
					t.Fatal("failed Begin error changed", e)
				}
			case <-time.After(time.Second):
				t.Fatal("Suspend joined whole transaction instead of stream")
			}
			if stream.Check() == nil || pending.Check() == nil {
				t.Fatal("failed Begin retained scope authority")
			}
			if _, e := os.Stat(filepath.Join(f.path, "continuation-v1", "key.bundle")); e != nil {
				t.Fatal("failed Begin purged durable key")
			}
			loaded, e := f.disk.Load()
			if e != nil || loaded.State != "active" {
				t.Fatal("failed Begin claimed durable logout")
			}
			stream.Close()
			pending.Close()
		})
	}
}
func TestOversizedCorruptKeyStartupKeepsLoginAndAllowsAttendedPurge(t *testing.T) {
	f := newFixture(t)
	l, _ := f.lease(t, context.Background(), nil)
	l.Close()
	if e := f.manager.Close(); e != nil {
		t.Fatal(e)
	}
	key := filepath.Join(f.path, "continuation-v1", "key.bundle")
	if e := os.WriteFile(key, []byte(strings.Repeat("damaged", 1000)), 0600); e != nil {
		t.Fatal(e)
	}
	disk, e := credentials.OpenStore(f.path)
	if e != nil {
		t.Fatal(e)
	}
	f.disk = disk
	f.open(t)
	if _, e = f.manager.Current(context.Background()); e != nil {
		t.Fatal("private cache corruption disabled stable login", e)
	}
	lease, e := f.store.Acquire(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = lease.Policy(atuin.Request{}, nil, nil); e == nil {
		t.Fatal("oversized damaged key accepted")
	}
	lease.Close()
	if e = f.manager.Logout(context.Background()); e != nil {
		t.Fatal("corrupt key parsing prevented authorized purge", e)
	}
	if _, e = os.Stat(key); !os.IsNotExist(e) {
		t.Fatal("corrupt key survived owned logout purge")
	}
}

func TestClockCheckedRangeAndSmallRollback(t *testing.T) {
	f := newFixture(t)
	l, _ := f.lease(t, context.Background(), nil)
	defer l.Close()
	original := f.now.Load()
	for _, delta := range []int64{-1, -60000} {
		f.now.Store(original + delta)
		if _, e := l.Policy(atuin.Request{}, nil, nil); e != nil {
			t.Fatal("allowed adjustment rejected", delta, e)
		}
		if f.store.floor.Load() != original {
			t.Fatal("small rollback extended TTL")
		}
	}
	for _, year := range []int{2019, 10000} {
		f.now.Store(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())
		if _, e := l.Policy(atuin.Request{}, nil, nil); e == nil {
			t.Fatal(fmt.Sprint("out-of-range year accepted ", year))
		}
	}
}
