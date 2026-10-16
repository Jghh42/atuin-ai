package siwcproof

import (
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
	r := Record{HostID: "urn:uuid:test", ClientID: "issued", AccessToken: "secret", Generation: 1}
	if s.save(r) != nil {
		t.Fatal("save failed")
	}
	got, e := s.load()
	if e != nil || got.HostID != r.HostID || got.AccessToken != r.AccessToken {
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
	base := privateTempDir(t)
	real := filepath.Join(base, "real")
	if os.Mkdir(real, 0700) != nil || os.Symlink(real, filepath.Join(base, "alias")) != nil {
		t.Fatal("setup failed")
	}
	if s, e := openStoreAt(filepath.Join(base, "alias", "child")); e == nil {
		s.close()
		t.Fatal("symlink directory accepted")
	}
	s, e := openStoreAt(filepath.Join(base, "state"))
	if e != nil {
		t.Fatal("open failed")
	}
	defer s.close()
	target := filepath.Join(base, "target")
	if os.WriteFile(target, []byte(`{}`), 0600) != nil {
		t.Fatal("setup failed")
	}
	p := filepath.Join(s.dir, "record.json")
	if os.Symlink(target, p) != nil {
		t.Fatal("symlink failed")
	}
	if _, e := s.load(); e == nil {
		t.Fatal("symlink read accepted")
	}
	if s.save(Record{}) == nil {
		t.Fatal("symlink write accepted")
	}
	os.Remove(p)
	if os.Link(target, p) != nil {
		t.Fatal("link failed")
	}
	if _, e := s.load(); e == nil {
		t.Fatal("hardlink accepted")
	}
}
func TestLockReleased(t *testing.T) {
	d := filepath.Join(privateTempDir(t), "private")
	s, e := openStoreAt(d)
	if e != nil {
		t.Fatal("open failed")
	}
	s.close()
	s, e = openStoreAt(d)
	if e != nil {
		t.Fatal("lock not released")
	}
	s.close()
}
