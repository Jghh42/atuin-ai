package setup

import (
	"context"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func TestPatchAtuinWritesClientToken(t *testing.T) {
	for _, text := range []string{"", "[ai]\napi_token = ''\nmodel = 'kept'\n", "[ai]\napi_token = 'old-secret' # comment\n"} {
		out, e := PatchAtuin([]byte(text), 17391, testToken)
		if e != nil {
			t.Fatal(e)
		}
		var cfg struct {
			AI struct {
				Token string `toml:"api_token"`
			} `toml:"ai"`
		}
		if _, e := toml.Decode(string(out), &cfg); e != nil || cfg.AI.Token != testToken {
			t.Fatalf("%q -> %q", text, out)
		}
	}
	for _, token := range []string{"", "secret", testToken + "\"\nendpoint = 'evil'", strings.ToUpper(testToken)} {
		if _, e := PatchAtuin(nil, 17391, token); e == nil {
			t.Fatalf("accepted token %q", token)
		}
	}
}

func TestWizardCreatesTokenBeforeServiceAndShowsManualConfig(t *testing.T) {
	// Decline the service and Atuin edit: manual instructions carry the token.
	w, a, _, events := fixture("", "")
	a.ready = true
	w.Port = 19222
	var configured string
	w.Configure = func(token string) error { configured = token; return nil }
	if e := w.Run(context.Background()); e != nil {
		t.Fatal(e)
	}
	tokenAt, closeAt := -1, -1
	for i, e := range *events {
		switch e {
		case "token":
			tokenAt = i
		case "close":
			closeAt = i
		}
	}
	if tokenAt < 0 || closeAt < tokenAt {
		t.Fatal("token must be created while the store is held", *events)
	}
	out := w.TTY.(*terminalFixture).String()
	if configured != "" || !strings.Contains(out, ManualAtuinConfig(19222, testToken)) {
		t.Fatalf("manual configuration missing:\n%s", out)
	}

	// Accepted edit receives the same token.
	w, a, _, _ = fixture("", "y")
	a.ready = true
	w.Configure = func(token string) error { configured = token; return nil }
	if e := w.Run(context.Background()); e != nil || configured != testToken {
		t.Fatalf("configure token=%q err=%v", configured, e)
	}

	// Declined login creates no token.
	w, _, _, events = fixture("n")
	if e := w.Run(context.Background()); e != nil || hasEvent(*events, "token") {
		t.Fatal(*events)
	}
}

func TestWizardTokenFailureMakesNoChanges(t *testing.T) {
	w, a, _, events := fixture("yes", "yes")
	a.ready = true
	a.tokenErr = credentials.ErrStore
	if e := w.Run(context.Background()); e != ErrSetup || hasEvent(*events, "install") || hasEvent(*events, "config") || !hasEvent(*events, "close") {
		t.Fatal(e, *events)
	}
}
