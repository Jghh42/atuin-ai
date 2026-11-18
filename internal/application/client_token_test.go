package application

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func TestOnlyServingRuntimesCreateClientToken(t *testing.T) {
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "credentials")
	status, e := NewStatus(path, config.Default(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if status.HTTP.ClientToken != "" {
		t.Fatal("status runtime loaded a serving token")
	}
	status.Close()
	if _, e := credentials.ReadClientToken(path); e != credentials.ErrNoClientToken {
		t.Fatal("status runtime created a token", e)
	}
	diagnostic, e := NewDiagnostic(path, config.Default(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := diagnostic.ClientToken(); e == nil {
		t.Fatal("read-only diagnostic runtime created a token")
	}
	diagnostic.Close()

	serving, e := New(path, config.Default(), nil)
	if e != nil {
		t.Fatal(e)
	}
	token := serving.HTTP.ClientToken
	if !credentials.ValidClientToken(token) {
		t.Fatal("serving runtime has no client token")
	}
	if got, e := serving.ClientToken(); e != nil || got != token {
		t.Fatal(e)
	}
	serving.Close()
	if shown, e := credentials.ReadClientToken(path); e != nil || shown != token {
		t.Fatal("serving token not persisted", e)
	}
}

func TestStateDirectoryResolvesSymlinkedConfigHome(t *testing.T) {
	root := t.TempDir()
	if e := os.Mkdir(filepath.Join(root, "dotfiles"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(root, "dotfiles"), filepath.Join(root, "config")); e != nil {
		t.Fatal(e)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)
	path, e := StateDirectory()
	real, _ := filepath.EvalSymlinks(root)
	if e != nil || path != filepath.Join(real, "dotfiles", "atuin-ai", "credentials") {
		t.Fatal("symlinked config directory not resolved", path, e)
	}
}
