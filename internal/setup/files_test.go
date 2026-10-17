//go:build linux || darwin

package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func privateRoot(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	if e := os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestFilesRefuseSymlinksLinksPermissionsAndRaces(t *testing.T) {
	files := DiskFiles{}
	for _, kind := range []string{"symlink", "hardlink", "directory", "writable", "ancestor-link", "ancestor-writable"} {
		t.Run(kind, func(t *testing.T) {
			root := privateRoot(t)
			path := filepath.Join(root, "config.toml")
			other := filepath.Join(root, "other")
			if e := os.WriteFile(other, []byte("original"), 0600); e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "symlink":
				if e := os.Symlink(other, path); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e := os.Link(other, path); e != nil {
					t.Fatal(e)
				}
			case "directory":
				if e := os.Mkdir(path, 0700); e != nil {
					t.Fatal(e)
				}
			case "writable":
				if e := os.WriteFile(path, []byte("original"), 0600); e != nil {
					t.Fatal(e)
				}
				if e := os.Chmod(path, 0666); e != nil {
					t.Fatal(e)
				}
			case "ancestor-link":
				if e := os.Symlink(root, filepath.Join(root, "link")); e != nil {
					t.Fatal(e)
				}
				path = filepath.Join(root, "link", "new")
			case "ancestor-writable":
				if e := os.Mkdir(filepath.Join(root, "unsafe"), 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Chmod(filepath.Join(root, "unsafe"), 0777); e != nil {
					t.Fatal(e)
				}
				path = filepath.Join(root, "unsafe", "new")
			}
			if _, e := files.Read(path, 100); e == nil {
				t.Fatal("unsafe read allowed")
			}
			if _, e := files.Replace(path, Snapshot{}, []byte("replacement"), 0600, true); e == nil {
				t.Fatal("unsafe write allowed")
			}
			b, _ := os.ReadFile(other)
			if string(b) != "original" {
				t.Fatal("unrelated target changed")
			}
		})
	}
	root := privateRoot(t)
	path := filepath.Join(root, "config.toml")
	first, e := files.Replace(path, Snapshot{}, []byte("first"), 0600, false)
	if e != nil {
		t.Fatal(e)
	}
	// Concurrent replacement of same bytes must still be rejected (inode check).
	secondPath := filepath.Join(root, "second")
	if e = os.WriteFile(secondPath, []byte("first"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(secondPath, path); e != nil {
		t.Fatal(e)
	}
	if _, e = files.Replace(path, first.After, []byte("wrong"), 0600, true); e == nil {
		t.Fatal("concurrent inode replacement accepted")
	}
	if e = files.Remove(path, first.After); e == nil {
		t.Fatal("removed concurrent replacement")
	}
	current, _ := files.Read(path, 100)
	if !bytes.Equal(current.Data, []byte("first")) {
		t.Fatal("current changed")
	}
	if _, e = files.Read(path, 1); e == nil {
		t.Fatal("read size bound ignored")
	}
	for _, p := range []string{"relative", "/a/../b", "/a//b", "/a\nfile", "/a\tfile", string([]byte{'/', 0xff})} {
		if _, e = files.Read(p, 100); e == nil {
			t.Fatal("ambiguous path accepted")
		}
	}
}
func TestFilesBackupExclusiveAndIdempotent(t *testing.T) {
	root := privateRoot(t)
	path := filepath.Join(root, "config.toml")
	files := DiskFiles{}
	old, e := files.Replace(path, Snapshot{}, []byte("old"), 0600, true)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		next, e := files.Replace(path, old.After, []byte("new"), 0600, true)
		if e != nil {
			t.Fatal(e)
		}
		old, e = files.Replace(path, next.After, []byte("old"), 0600, true)
		if e != nil {
			t.Fatal(e)
		}
	}
	names, _ := filepath.Glob(filepath.Join(root, ".atuin-ai-backup-*"))
	if len(names) != 4 {
		t.Fatal("backup overwritten", names)
	}
	if _, e = files.Replace(path, old.After, old.After.Data, 0600, true); e != nil {
		t.Fatal(e)
	}
	names, _ = filepath.Glob(filepath.Join(root, ".atuin-ai-backup-*"))
	if len(names) != 4 {
		t.Fatal("noop backed up")
	}
}
