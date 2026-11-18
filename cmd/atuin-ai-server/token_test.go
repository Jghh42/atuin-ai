package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/application"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func TestTokenShowAndRotate(t *testing.T) {
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	deps := commandDeps{state: filepath.Join(root, "app-state"), transport: authTransport(func(*http.Request) (*http.Response, error) {
		t.Error("token command made a network request")
		return nil, credentials.ErrUnavailable
	})}
	show := func() string {
		t.Helper()
		var out bytes.Buffer
		if e := runWith(context.Background(), []string{"token", "show"}, &out, deps); e != nil {
			t.Fatal(e)
		}
		token := strings.TrimSuffix(out.String(), "\n")
		if !credentials.ValidClientToken(token) {
			t.Fatalf("show printed %q", out.String())
		}
		return token
	}
	// First show creates the token; later shows return the same one.
	first := show()
	if show() != first {
		t.Fatal("token changed between shows")
	}

	// A running server holds the lifetime lock: show still works, rotate refuses.
	running, e := application.New(deps.state, config.Default(), deps.transport)
	if e != nil {
		t.Fatal(e)
	}
	if running.HTTP.ClientToken != first {
		t.Fatal("serving runtime loaded a different token")
	}
	if show() != first {
		t.Fatal("show while running")
	}
	if e := runWith(context.Background(), []string{"token", "rotate"}, &bytes.Buffer{}, deps); e != credentials.ErrLocked {
		t.Fatal("rotate bypassed the running server", e)
	}
	running.Close()

	var out bytes.Buffer
	if e := runWith(context.Background(), []string{"token", "rotate"}, &out, deps); e != nil {
		t.Fatal(e)
	}
	rotated := show()
	if rotated == first || !strings.Contains(out.String(), rotated) || !strings.Contains(out.String(), "api_token") {
		t.Fatal("rotate did not replace and report the token")
	}

	for _, args := range [][]string{{"token"}, {"token", "print"}, {"token", "show", "extra"}} {
		if e := runWith(context.Background(), args, &bytes.Buffer{}, deps); e == nil || !strings.Contains(e.Error(), "usage") {
			t.Fatal("bad usage accepted", args)
		}
	}
}
