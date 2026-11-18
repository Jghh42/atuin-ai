package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientTokenCreatedOnceAndPersisted(t *testing.T) {
	s, path := testStore(t)
	if _, e := ReadClientToken(path); !errors.Is(e, ErrNoClientToken) {
		t.Fatal("token existed before creation", e)
	}
	token, e := s.ClientToken()
	if e != nil || !ValidClientToken(token) {
		t.Fatal(e)
	}
	again, e := s.ClientToken()
	if e != nil || again != token {
		t.Fatal("token not stable within a store lifetime")
	}
	st, e := os.Lstat(filepath.Join(path, clientTokenFile))
	if e != nil || st.Mode() != 0600 {
		t.Fatal("token file must be private and regular", e)
	}
	// Lock-free read works while this store holds the lifetime lock.
	if shown, e := ReadClientToken(path); e != nil || shown != token {
		t.Fatal("lock-free read", e)
	}
	s.Close()
	reopened, e := OpenStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if after, e := reopened.ClientToken(); e != nil || after != token {
		t.Fatal("token not persisted across restart")
	}
}

func TestClientTokenRotation(t *testing.T) {
	s, path := testStore(t)
	old, e := s.ClientToken()
	if e != nil {
		t.Fatal(e)
	}
	rotated, e := s.RotateClientToken()
	if e != nil || rotated == old || !ValidClientToken(rotated) {
		t.Fatal("rotation did not replace the token", e)
	}
	if shown, e := ReadClientToken(path); e != nil || shown != rotated {
		t.Fatal("rotation not durable", e)
	}
	names, _ := os.ReadDir(path)
	for _, n := range names {
		if strings.HasPrefix(n.Name(), ".record-") {
			t.Fatal("temporary file left behind")
		}
	}
}

func TestClientTokenRefusesUnsafeOrCorruptFiles(t *testing.T) {
	for name, prepare := range map[string]func(t *testing.T, path string){
		"corrupt": func(t *testing.T, path string) {
			if e := os.WriteFile(filepath.Join(path, clientTokenFile), []byte("not-a-token\n"), 0600); e != nil {
				t.Fatal(e)
			}
		},
		"group readable": func(t *testing.T, path string) {
			if e := os.WriteFile(filepath.Join(path, clientTokenFile), []byte(ClientTokenPrefix+strings.Repeat("a", 64)+"\n"), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.Chmod(filepath.Join(path, clientTokenFile), 0640); e != nil {
				t.Fatal(e)
			}
		},
		"symlink": func(t *testing.T, path string) {
			target := filepath.Join(t.TempDir(), "elsewhere")
			if e := os.WriteFile(target, []byte(ClientTokenPrefix+strings.Repeat("a", 64)+"\n"), 0600); e != nil {
				t.Fatal(e)
			}
			if e := os.Symlink(target, filepath.Join(path, clientTokenFile)); e != nil {
				t.Fatal(e)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, path := testStore(t)
			prepare(t, path)
			if _, e := s.ClientToken(); !errors.Is(e, ErrStore) {
				t.Fatal("unsafe token file accepted", e)
			}
			if _, e := ReadClientToken(path); !errors.Is(e, ErrStore) {
				t.Fatal("unsafe token file shown", e)
			}
			rotated, e := s.RotateClientToken()
			if name == "corrupt" {
				// A private regular file with bad contents is recoverable by rotation.
				if shown, err := ReadClientToken(path); e != nil || err != nil || shown != rotated {
					t.Fatal("rotation did not recover a corrupt token", e, err)
				}
			} else if !errors.Is(e, ErrStore) {
				t.Fatal("rotation replaced an unsafe destination", e)
			}
		})
	}
}

func TestClientTokenDiagnosticStoreNeverCreates(t *testing.T) {
	s, path := testStore(t)
	s.Close()
	d, e := OpenDiagnosticStore(path)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	if _, e := d.ClientToken(); !errors.Is(e, ErrStore) {
		t.Fatal("read-only store created a token", e)
	}
	if _, e := os.Lstat(filepath.Join(path, clientTokenFile)); !os.IsNotExist(e) {
		t.Fatal("token file created")
	}
	if _, e := ReadClientToken(filepath.Join(path, "missing")); !errors.Is(e, ErrNoClientToken) {
		t.Fatal("missing directory", e)
	}
}
