//go:build linux || darwin

package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestExplainStoreNamesTheFailingPath(t *testing.T) {
	defer syscall.Umask(syscall.Umask(077))
	for name, tc := range map[string]struct {
		prepare func(t *testing.T, root string) string
		want    string
	}{
		"symlinked ancestor": {func(t *testing.T, root string) string {
			if e := os.Mkdir(filepath.Join(root, "real"), 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); e != nil {
				t.Fatal(e)
			}
			return filepath.Join(root, "link", "credentials")
		}, "link is a symlink"},
		"group-writable ancestor": {func(t *testing.T, root string) string {
			if e := os.Mkdir(filepath.Join(root, "shared"), 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.Chmod(filepath.Join(root, "shared"), 0775); e != nil {
				t.Fatal(e)
			}
			return filepath.Join(root, "shared", "credentials")
		}, "chmod go-w"},
		"loose final directory": {func(t *testing.T, root string) string {
			if e := os.Mkdir(filepath.Join(root, "credentials"), 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.Chmod(filepath.Join(root, "credentials"), 0755); e != nil {
				t.Fatal(e)
			}
			return filepath.Join(root, "credentials")
		}, "chmod 700"},
		"loose lock file": {func(t *testing.T, root string) string {
			path := filepath.Join(root, "credentials")
			if e := os.Mkdir(path, 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(path, "lock"), nil, 0644); e != nil {
				t.Fatal(e)
			}
			if e := os.Chmod(filepath.Join(path, "lock"), 0644); e != nil {
				t.Fatal(e)
			}
			return path
		}, "lock must be a regular 0600 file"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if e := os.Chmod(root, 0700); e != nil {
				t.Fatal(e)
			}
			path := tc.prepare(t, root)
			if _, e := OpenStore(path); !errors.Is(e, ErrStore) {
				t.Fatal("unsafe store opened", e)
			}
			e := ExplainStore(path)
			if !errors.Is(e, ErrStore) || !strings.Contains(e.Error(), tc.want) {
				t.Fatalf("got %v, want mention of %q", e, tc.want)
			}
		})
	}
	// A healthy store has nothing to explain.
	_, path := testStore(t)
	if e := ExplainStore(path); e != ErrStore {
		t.Fatal("healthy store explained", e)
	}
}
