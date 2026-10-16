package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/application"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

type authTransport func(*http.Request) (*http.Response, error)

func (f authTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAuthCommandsPersistedLoginDiscoveryFailureStatusAndLogout(t *testing.T) {
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("CHAT_CONFIG", "")
	path := filepath.Join(root, "app-state")
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "private-test-account"}, "exp": now.Add(time.Hour).Unix()})
	token := "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".c2ln"
	displayed := 0
	deps := commandDeps{state: path, display: func(p credentials.DevicePrompt) error {
		displayed++
		if p.UserCode != "PRIVATE-CODE" {
			t.Error("incorrect interactive prompt")
		}
		return nil
	}, transport: authTransport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			body = `{"device_auth_id":"private-device","user_code":"PRIVATE-CODE","interval":1}`
		case "/api/accounts/deviceauth/token":
			body = `{"authorization_code":"private-code","code_verifier":"` + strings.Repeat("v", 43) + `"}`
		case "/oauth/token":
			b, _ := json.Marshal(map[string]any{"access_token": token, "refresh_token": "private-refresh", "expires_in": 3600})
			body = string(b)
		case "/backend-api/codex/models":
			body = `{"models":[{"slug":"unproven-model","display_name":"Unproven","visibility":"list","priority":0}]}`
		default:
			t.Error("unexpected HTTP endpoint")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	check := func(command, want string) {
		t.Helper()
		var output bytes.Buffer
		if e := runWith(context.Background(), []string{"auth", command}, &output, deps); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(output.String(), want) {
			t.Fatal("incorrect command result")
		}
		for _, secret := range []string{path, token, "private-test-account", "private-refresh", "PRIVATE-CODE", "private-device", "private-code"} {
			if strings.Contains(output.String(), secret) {
				t.Fatal("command secret disclosure")
			}
		}
	}
	check("status", "Logged out")
	check("login", "Login saved successfully; models unavailable")
	if displayed != 1 {
		t.Fatal("code not restricted to interactive display")
	}
	check("status", "Logged in")
	running, e := application.New(path, config.Default(), deps.transport)
	if e != nil {
		t.Fatal(e)
	}
	if e := runWith(context.Background(), []string{"auth", "logout"}, io.Discard, deps); e != credentials.ErrStore {
		t.Fatal("separate command bypassed lifetime lock", e)
	}
	running.Close()
	check("logout", "Local credentials deleted")
	check("status", "Logged out")
}
func TestAuthArgumentsNeverEchoValues(t *testing.T) {
	for _, args := range [][]string{{"auth", "login", "private-value"}, {"auth", "status", "--token", "private-value"}, {"auth", "private-value"}} {
		var output bytes.Buffer
		e := run(context.Background(), args, &output)
		if e == nil || strings.Contains(e.Error(), "private-value") || strings.Contains(output.String(), "private-value") {
			t.Fatal("unsafe auth argument handling")
		}
	}
}
