package codexproof

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, p := range []string{filepath.Dir(dir), dir} {
		if err := os.Chmod(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func TestPrivateStoreLockAndPermissions(t *testing.T) {
	d := filepath.Join(privateTempDir(t), "state")
	s, e := openStoreAt(d)
	if e != nil {
		t.Fatal("open failed")
	}
	defer s.close()
	if second, e := openStoreAt(d); e == nil {
		second.close()
		t.Fatal("concurrent lock accepted")
	}
	r := Record{AccountID: "account", AccessToken: "secret", Generation: 1}
	if s.save(r) != nil {
		t.Fatal("save failed")
	}
	got, e := s.load()
	if e != nil || got.AccountID != r.AccountID || got.AccessToken != r.AccessToken {
		t.Fatal("round trip failed")
	}
	for _, p := range []string{d, filepath.Join(d, "record.json"), filepath.Join(d, "lock")} {
		st, e := os.Stat(p)
		if e != nil {
			t.Fatal("stat failed")
		}
		want := os.FileMode(0600)
		if p == d {
			want = 0700
		}
		if st.Mode().Perm() != want {
			t.Fatal("unsafe permissions")
		}
	}
	if os.Chmod(filepath.Join(d, "record.json"), 0644) != nil {
		t.Fatal("chmod failed")
	}
	if _, e := s.load(); e == nil {
		t.Fatal("public secret accepted")
	}
	if s.save(r) == nil {
		t.Fatal("unsafe file replaced")
	}
}
func TestSymlinkAndHardlinkRejected(t *testing.T) {
	for _, name := range []string{"record.json", "lock"} {
		for _, kind := range []string{"symlink", "hardlink"} {
			t.Run(name+kind, func(t *testing.T) {
				base := privateTempDir(t)
				d := filepath.Join(base, "state")
				if os.Mkdir(d, 0700) != nil {
					t.Fatal("mkdir")
				}
				target := filepath.Join(base, "target")
				if os.WriteFile(target, []byte(`{}`), 0600) != nil {
					t.Fatal("write")
				}
				p := filepath.Join(d, name)
				var e error
				if kind == "symlink" {
					e = os.Symlink(target, p)
				} else {
					e = os.Link(target, p)
				}
				if e != nil {
					t.Fatal(e)
				}
				s, e := openStoreAt(d)
				if name == "lock" {
					if e == nil {
						s.close()
						t.Fatal("unsafe lock accepted")
					}
					return
				}
				if e != nil {
					t.Fatal(e)
				}
				defer s.close()
				if _, e := s.load(); e == nil {
					t.Fatal("unsafe record read")
				}
				if s.save(Record{}) == nil {
					t.Fatal("unsafe record replaced")
				}
			})
		}
	}
}
func TestUnsafeAncestryRejected(t *testing.T) {
	base := privateTempDir(t)
	real := filepath.Join(base, "real")
	if os.Mkdir(real, 0700) != nil || os.Symlink(real, filepath.Join(base, "alias")) != nil {
		t.Fatal("setup")
	}
	if s, e := openStoreAt(filepath.Join(base, "alias", "child")); e == nil {
		s.close()
		t.Fatal("symlink ancestry accepted")
	}
	if os.Chmod(real, 0777) != nil {
		t.Fatal("chmod")
	}
	if s, e := openStoreAt(filepath.Join(real, "state")); e == nil {
		s.close()
		t.Fatal("writable ancestry accepted")
	}
}
func TestPrivateOrphanCleanupUnderLock(t *testing.T) {
	d := filepath.Join(privateTempDir(t), "state")
	s, err := openStoreAt(d)
	if err != nil {
		t.Fatal(err)
	}
	s.close()
	orphan := filepath.Join(d, ".record-crash")
	if err := os.WriteFile(orphan, []byte("synthetic-private"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = openStoreAt(d)
	if err != nil {
		t.Fatal(err)
	}
	s.close()
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Fatal("private temporary record retained")
	}
	target := filepath.Join(d, "unrelated")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, orphan); err != nil {
		t.Fatal(err)
	}
	if s, err := openStoreAt(d); err == nil {
		s.close()
		t.Fatal("symlink orphan accepted")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("symlink target modified")
	}
}

func TestLockReleasedAndCancelledContext(t *testing.T) {
	testHome(t)
	s, e := openStore()
	if e != nil {
		t.Fatal(e)
	}
	s.close()
	s, e = openStore()
	if e != nil {
		t.Fatal("lock not released")
	}
	s.close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if WithRecord(ctx, func(Record) error { t.Error("cancelled callback invoked"); return nil }) == nil || Logout(ctx) == nil || Refresh(ctx) == nil {
		t.Fatal("cancelled operation accepted")
	}
}
