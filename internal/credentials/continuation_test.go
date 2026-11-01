package credentials

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestContinuationCapabilityRejectsUnsafeEntriesAndEscapes(t *testing.T) {
	for _, kind := range []string{"mode", "symlink", "hardlink", "fifo", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			s, path := testStore(t)
			d, e := s.OpenContinuationDirectory()
			if e != nil {
				t.Fatal(e)
			}
			defer d.Close()
			name := "00000000000000000000000000000000.sealed"
			p := filepath.Join(path, "continuation-v1", name)
			switch kind {
			case "mode":
				e = os.WriteFile(p, []byte("x"), 0644)
			case "symlink":
				e = os.Symlink("../record.json", p)
			case "hardlink":
				e = os.WriteFile(p, []byte("x"), 0600)
				if e == nil {
					e = os.Link(p, p+"extra")
				}
			case "fifo":
				e = unix.Mkfifo(p, 0600)
			case "unknown":
				e = os.WriteFile(filepath.Join(path, "continuation-v1", "unknown"), nil, 0600)
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = d.Scan(); e == nil {
				t.Fatal("unsafe directory accepted")
			}
			if e = d.Remove(name); e == nil && kind != "unknown" {
				t.Fatal("unsafe file deleted")
			}
			for _, escape := range []string{"../record.json", "/tmp/key.bundle", "key.bundle/../record.json", "record.json", ".pending-bad"} {
				if _, _, e = d.Read(escape, 4096); e == nil {
					t.Fatal("read escaped fixed namespace")
				}
				if e = d.Atomic(context.Background(), escape, nil, nil); e == nil {
					t.Fatal("write escaped namespace")
				}
			}
		})
	}
}
func TestContinuationChildModesDiagnosticAndAnchoring(t *testing.T) {
	for _, mode := range []os.FileMode{0755, 0710, 0777} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			s, path := testStore(t)
			p := filepath.Join(path, "continuation-v1")
			if e := os.Mkdir(p, mode); e != nil {
				t.Fatal(e)
			}
			if _, e := s.OpenContinuationDirectory(); e == nil {
				t.Fatal("unsafe child mode")
			}
		})
	}
	s, path := testStore(t)
	d, e := s.OpenContinuationDirectory()
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	moved := path + "-moved"
	if e = os.Rename(path, moved); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e = d.Atomic(context.Background(), "key.bundle", []byte("private"), nil); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(moved, "continuation-v1", "key.bundle")); e != nil {
		t.Fatal("lost descriptor anchor")
	}
	if _, e = os.Stat(filepath.Join(path, "continuation-v1")); !os.IsNotExist(e) {
		t.Fatal("re-resolved user path")
	}
	// A diagnostic store never creates/acquires continuation capability.
	s.Close()
	d.Close()
	diagnostic, e := OpenDiagnosticStore(moved)
	if e != nil {
		t.Fatal(e)
	}
	defer diagnostic.Close()
	if _, e = diagnostic.OpenContinuationDirectory(); e == nil {
		t.Fatal("diagnostic acquired mutable continuation directory")
	}
}
func TestContinuationScanIsBoundedAndPendingBytesCount(t *testing.T) {
	s, path := testStore(t)
	d, e := s.OpenContinuationDirectory()
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	for j := 0; j < 133; j++ {
		name := fmt.Sprintf(".pending-%032x", j)
		if e = os.WriteFile(filepath.Join(path, "continuation-v1", name), []byte("123"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = d.Scan(); e == nil {
		t.Fatal("133-entry directory accepted")
	}
	if e = os.Remove(filepath.Join(path, "continuation-v1", fmt.Sprintf(".pending-%032x", 132))); e != nil {
		t.Fatal(e)
	}
	entries, e := d.Scan()
	if e != nil || len(entries) != 132 {
		t.Fatal("132-entry control", e)
	}
	var bytes int64
	for _, v := range entries {
		bytes += v.Bytes()
	}
	if bytes != 396 {
		t.Fatal("pending bytes ignored")
	}
}
