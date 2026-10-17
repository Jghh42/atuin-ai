package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type forbiddenTransport struct{ t *testing.T }

func (f forbiddenTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Fatal("version opened network")
	return nil, nil
}

func TestVersionHasNoRuntimeSideEffects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CHAT_CONFIG", filepath.Join(dir, "missing-explicit-config"))
	for _, arg := range []string{"version", "--version"} {
		var out bytes.Buffer
		err := runWith(context.Background(), []string{arg}, &out, commandDeps{
			state: filepath.Join(dir, "must-not-create"), transport: forbiddenTransport{t},
		})
		if err != nil || !strings.Contains(out.String(), "revision=") || !strings.Contains(out.String(), "target=") {
			t.Fatalf("version: %q %v", out.String(), err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("version touched private state: %v %v", entries, err)
	}
	if err := run(context.Background(), []string{"version", "extra"}, &bytes.Buffer{}); err == nil {
		t.Fatal("version accepted arguments")
	}
}
