package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestStoreLockChild(t *testing.T) {
	path := os.Getenv("ATUIN_TEST_LOCK_PATH")
	if path == "" {
		return
	}
	s, e := OpenStore(path)
	if e == nil {
		s.Close()
		t.Fatal("child acquired parent's lifetime lock")
	}
}
func TestStoreCrossProcessLifetimeLock(t *testing.T) {
	_, path := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStoreLockChild$")
	cmd.Env = append(os.Environ(), "ATUIN_TEST_LOCK_PATH="+path)
	if e := cmd.Run(); e != nil {
		t.Fatal("cross-process lock check failed", e)
	}
}
func TestStorePrivateLockAndRestart(t *testing.T) {
	s, path := testStore(t)
	r := activeRecord(testNow.Add(time.Hour))
	seed(t, s, r)
	for _, name := range []string{"record.json", "lock"} {
		st, e := os.Stat(filepath.Join(path, name))
		if e != nil || st.Mode().Perm() != 0600 {
			t.Fatal("file not private")
		}
	}
	st, e := os.Stat(path)
	if e != nil || st.Mode().Perm() != 0700 {
		t.Fatal("directory not private")
	}
	lockBefore, _ := os.Stat(filepath.Join(path, "lock"))
	if second, e := OpenStore(path); e == nil {
		second.Close()
		t.Fatal("concurrent writer admitted")
	}
	loaded, e := s.Load()
	if e != nil || loaded.Access != r.Access || loaded.Generation != 1 {
		t.Fatal("wrong saved record", e)
	}
	s.Close()
	again, e := OpenStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	loaded, e = again.Load()
	if e != nil || loaded.Refresh != r.Refresh {
		t.Fatal("restart load failed", e)
	}
	lockAfter, _ := os.Stat(filepath.Join(path, "lock"))
	if !os.SameFile(lockBefore, lockAfter) {
		t.Fatal("lock inode replaced")
	}
}
func TestStoreRejectsUnsafeFilesAndAncestry(t *testing.T) {
	for _, kind := range []string{"filemode", "symlink", "hardlink", "fifo", "directory", "owner"} {
		t.Run(kind, func(t *testing.T) {
			s, path := testStore(t)
			target := filepath.Join(path, "record.json")
			switch kind {
			case "filemode":
				_ = os.WriteFile(target, []byte(`{}`), 0644)
			case "symlink":
				_ = os.Symlink(filepath.Join(t.TempDir(), "outside"), target)
			case "hardlink":
				outside := filepath.Join(t.TempDir(), "outside")
				_ = os.WriteFile(outside, []byte(`{}`), 0600)
				_ = os.Link(outside, target)
			case "fifo":
				if e := unix.Mkfifo(target, 0600); e != nil {
					t.Fatal(e)
				}
			case "directory":
				_ = os.Mkdir(target, 0700)
			case "owner":
				if os.Geteuid() != 0 {
					t.Skip("owner mismatch needs privilege; all opens check euid")
				}
				_ = os.WriteFile(target, []byte(`{}`), 0600)
				if e := os.Chown(target, 1, -1); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := s.Load(); e == nil {
				t.Fatal("unsafe record accepted")
			}
			r := activeRecord(testNow.Add(time.Hour))
			if e := s.atomic(context.Background(), "record.json", r); e == nil {
				t.Fatal("unsafe destination accepted")
			}
		})
	}
	for _, kind := range []string{"symlink-ancestor", "writable-ancestor", "wide-final", "symlink-lock", "hardlink-lock", "lockmode"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if e := os.Chmod(root, 0700); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(root, "private")
			switch kind {
			case "symlink-ancestor":
				other := t.TempDir()
				_ = os.Symlink(other, filepath.Join(root, "link"))
				path = filepath.Join(root, "link", "private")
			case "writable-ancestor":
				_ = os.Chmod(root, 0777)
			case "wide-final":
				_ = os.Mkdir(path, 0755)
			default:
				_ = os.Mkdir(path, 0700)
				lock := filepath.Join(path, "lock")
				outside := filepath.Join(root, "outside")
				_ = os.WriteFile(outside, nil, 0600)
				if kind == "symlink-lock" {
					_ = os.Symlink(outside, lock)
				} else if kind == "hardlink-lock" {
					_ = os.Link(outside, lock)
				} else {
					_ = os.WriteFile(lock, nil, 0644)
				}
			}
			if s, e := OpenStore(path); e == nil {
				s.Close()
				t.Fatal("unsafe root/lock accepted")
			}
		})
	}
}
func TestStoreCorruptionAndBounds(t *testing.T) {
	r := activeRecord(testNow.Add(time.Hour))
	valid, _ := json.Marshal(r)
	variants := [][]byte{[]byte(strings.Replace(string(valid), `"schema":1`, `"Schema":1`, 1)), []byte(strings.Replace(string(valid), `"auth_epoch":1,`, "", 1)), []byte(`{`), []byte(`null`), []byte(strings.Repeat("x", maxRecordBytes+1)), append(valid, []byte(`{}`)...), []byte(strings.Replace(string(valid), `"schema":1`, `"schema":1,"schema":1`, 1)), []byte(strings.Replace(string(valid), `"provider":"codex"`, `"provider":"other"`, 1)), []byte(strings.Replace(string(valid), ClientID, "unreviewed", 1))}
	for _, mutate := range []func(*Record){func(r *Record) { r.Generation = 0 }, func(r *Record) { r.AuthEpoch = r.Generation + 1 }, func(r *Record) { r.Account = "other" }, func(r *Record) { r.ExpiresAt = r.IssuedAt }, func(r *Record) { r.Refresh = "" }, func(r *Record) { r.State = "unexpected" }, func(r *Record) { r.SavedAt = time.Time{} }, func(r *Record) { r.State = "logged_out" }} {
		bad := r
		mutate(&bad)
		b, _ := json.Marshal(bad)
		variants = append(variants, b)
	}
	for i, b := range variants {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s, path := testStore(t)
			if e := os.WriteFile(filepath.Join(path, "record.json"), b, 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := s.Load(); e == nil {
				t.Fatal("corruption accepted")
			}
		})
	}
}
func TestStoreAtomicFaultsAndIntentRestartSafety(t *testing.T) {
	for _, stage := range []string{"write", "fsync", "rename", "dirsync"} {
		for _, target := range []string{"intent.json", "record.json"} {
			t.Run(target+"-"+stage, func(t *testing.T) {
				s, path := testStore(t)
				old := activeRecord(testNow.Add(time.Hour))
				seed(t, s, old)
				intent, _ := next(old, false, testNow)
				if target == "record.json" {
					if e := s.Begin(context.Background(), intent); e != nil {
						t.Fatal(e)
					}
				}
				s.fault = func(op string) error {
					if op == target+"."+stage {
						return errors.New("injected")
					}
					return nil
				}
				var e error
				if target == "intent.json" {
					e = s.Begin(context.Background(), intent)
				} else {
					new := old
					new.Generation++
					new.Refresh = "rotated-replacement"
					e = s.Commit(context.Background(), new)
				}
				if e == nil {
					t.Fatal("fault not propagated")
				}
				s.Close()
				again, e := OpenStore(path)
				if e != nil {
					t.Fatal(e)
				}
				defer again.Close()
				loaded, e := again.Load()
				if e != nil {
					t.Fatal(e)
				}
				if target == "record.json" && loaded.State != "unusable" {
					t.Fatal("replacement/old session exposed after failed persistence")
				}
				if loaded.State == "unusable" && (loaded.Access != "" || loaded.Refresh != "" || loaded.Account != "") {
					t.Fatal("intent contains secrets")
				}
			})
		}
	}
}
func TestStoreTombstoneOrphansAndCounters(t *testing.T) {
	s, path := testStore(t)
	old := activeRecord(testNow.Add(time.Hour))
	seed(t, s, old)
	intent, _ := next(old, false, testNow)
	if e := s.Begin(context.Background(), intent); e != nil {
		t.Fatal(e)
	}
	logout := intent
	logout.State = "logged_out"
	if e := s.Commit(context.Background(), logout); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(path, "record.json"))
	if strings.Contains(string(b), old.Access) || strings.Contains(string(b), old.Refresh) || strings.Contains(string(b), old.Account) {
		t.Fatal("logout retains secrets")
	}
	if e := s.Begin(context.Background(), intent); e == nil {
		t.Fatal("nonmonotonic generation accepted")
	}
	if e := os.WriteFile(filepath.Join(path, ".record-orphan"), []byte("synthetic orphan"), 0600); e != nil {
		t.Fatal(e)
	}
	s.Close()
	again, e := OpenStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer again.Close()
	if _, e := os.Lstat(filepath.Join(path, ".record-orphan")); !os.IsNotExist(e) {
		t.Fatal("orphan retained")
	}
	loaded, e := again.Load()
	if e != nil || loaded.Generation != 2 || loaded.AuthEpoch != 1 || loaded.State != "logged_out" {
		t.Fatal("tombstone counters lost")
	}
}
func TestStoreDescriptorRootCannotBeRedirected(t *testing.T) {
	s, path := testStore(t)
	moved := path + "-moved"
	if e := os.Rename(path, moved); e != nil {
		t.Fatal(e)
	}
	outside := t.TempDir()
	if e := os.Symlink(outside, path); e != nil {
		t.Fatal(e)
	}
	seed(t, s, activeRecord(testNow.Add(time.Hour)))
	if _, e := os.Stat(filepath.Join(outside, "record.json")); !os.IsNotExist(e) {
		t.Fatal("store followed replaced ancestor")
	}
	if _, e := os.Stat(filepath.Join(moved, "record.json")); e != nil {
		t.Fatal("store lost directory capability")
	}
}
func TestStoreUnsafeOrphanNotFollowed(t *testing.T) {
	s, path := testStore(t)
	outside := filepath.Join(t.TempDir(), "keep")
	_ = os.WriteFile(outside, []byte("keep"), 0600)
	_ = os.Symlink(outside, filepath.Join(path, ".record-link"))
	s.Close()
	if again, e := OpenStore(path); e == nil {
		again.Close()
		t.Fatal("unsafe orphan accepted")
	}
	b, e := os.ReadFile(outside)
	if e != nil || string(b) != "keep" {
		t.Fatal("orphan cleanup touched outside file")
	}
}
