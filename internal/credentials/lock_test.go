//go:build linux || darwin

package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOnlyActualContentionHasLockedSentinel(t *testing.T) {
	first, path := testStore(t)
	if _, err := OpenStore(path); !errors.Is(err, ErrLocked) || !errors.Is(err, ErrStore) || err.Error() != ErrStore.Error() {
		t.Fatal("real contention not distinguished")
	}
	first.Close()
	// A bad private lock file is not evidence of a running managed service.
	if err := os.Chmod(filepath.Join(path, "lock"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil || errors.Is(err, ErrLocked) {
		t.Fatal("permission failure classified as contention")
	}
	if err := os.Chmod(filepath.Join(path, "lock"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "record.json"), []byte("synthetic-corrupt-record"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Load(); err == nil || errors.Is(err, ErrLocked) {
		t.Fatal("corruption classified as contention")
	}
}
