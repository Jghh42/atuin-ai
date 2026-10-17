package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

func seedDiagnosticSession(t *testing.T, path string, expiry time.Time, pending bool) {
	t.Helper()
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "private-diagnostic-account"}, "exp": expiry.Unix()})
	token := "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".c2ln"
	intent := credentials.Record{Schema: 1, Provider: "codex", Registration: credentials.ClientID, State: "unusable", Generation: 1, AuthEpoch: 1, SavedAt: now}
	record := intent
	record.State, record.Account, record.Access, record.Refresh = "active", "private-diagnostic-account", token, "private-diagnostic-refresh"
	record.IssuedAt, record.ExpiresAt = now.Add(-time.Hour), expiry.Truncate(time.Second)
	s, err := credentials.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Begin(context.Background(), intent) != nil || s.Commit(context.Background(), record) != nil {
		t.Fatal("synthetic diagnostic seed failed")
	}
	if pending {
		intent.Generation++
		if s.Begin(context.Background(), intent) != nil {
			t.Fatal("synthetic pending intent failed")
		}
	}
}

func TestAuthDiagnoseSavedSessionNoRefreshStableJSON(t *testing.T) {
	for _, tc := range []struct {
		name, config, want                string
		remaining                         time.Duration
		pending, missing, locked, corrupt bool
		status, requests                  int
	}{
		{name: "success no config", remaining: time.Hour, want: "success", requests: 1},
		{name: "valid override", config: `default_model = "gpt-6.1-sol"`, remaining: time.Hour, want: "success", requests: 1},
		{name: "invalid override", config: `default_model = "PRIVATE-OVERRIDE"`, remaining: time.Hour, want: "invalid_default_override", requests: 1},
		{name: "invalid config", config: `PRIVATE-CONFIG = "PRIVATE-VALUE"`, remaining: time.Hour, want: "configuration_unavailable"},
		{name: "expired", remaining: -time.Minute, want: "refresh_required"},
		{name: "near expiry", remaining: 30 * time.Second, want: "refresh_required"},
		{name: "pending intent", remaining: time.Hour, pending: true, want: "credential_unavailable"},
		{name: "no existing store", missing: true, want: "credential_unavailable"},
		{name: "running server lock", remaining: time.Hour, locked: true, want: "credential_unavailable"},
		{name: "corrupt store", remaining: time.Hour, corrupt: true, want: "credential_unavailable"},
		{name: "unauthorized", remaining: time.Hour, status: 401, want: "http_unauthorized", requests: 1},
		{name: "forbidden", remaining: time.Hour, status: 403, want: "http_forbidden", requests: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if os.Chmod(root, 0700) != nil {
				t.Fatal("private fixture failed")
			}
			t.Setenv("HOME", root)
			t.Setenv("XDG_CONFIG_HOME", root)
			t.Setenv("CHAT_CONFIG", "")
			path := filepath.Join(root, "private-app-state")
			if !tc.missing {
				seedDiagnosticSession(t, path, time.Now().UTC().Add(tc.remaining), tc.pending)
			}
			if tc.config != "" {
				cfg := filepath.Join(root, "config.toml")
				if os.WriteFile(cfg, []byte(tc.config), 0600) != nil {
					t.Fatal("config fixture failed")
				}
				t.Setenv("CHAT_CONFIG", cfg)
			}
			if tc.corrupt && os.WriteFile(filepath.Join(path, "record.json"), []byte("PRIVATE-CORRUPT-RECORD"), 0600) != nil {
				t.Fatal("corrupt fixture failed")
			}
			var lock *credentials.DiskStore
			if tc.locked {
				var err error
				lock, err = credentials.OpenStore(path)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
			}
			var before []byte
			if !tc.missing {
				before, _ = os.ReadFile(filepath.Join(path, "record.json"))
			}
			requests := 0
			deps := commandDeps{state: path, transport: authTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.String() != "https://chatgpt.com/backend-api/codex/models?client_version=0.134.0" || r.Method != "GET" {
					t.Error("diagnostic attempted OAuth/inference/unreviewed endpoint")
				}
				if r.Header.Get("Authorization") == "" || r.Header.Get("ChatGPT-Account-Id") != "private-diagnostic-account" {
					t.Error("missing atomic session identity")
				}
				status := tc.status
				if status == 0 {
					status = 200
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":"gpt-6.1-sol","display_name":"PRIVATE-NAME","description":"PRIVATE-DESCRIPTION","visibility":"list","priority":0}]}`))}, nil
			})}
			var output bytes.Buffer
			if err := runWith(context.Background(), []string{"auth", "diagnose"}, &output, deps); err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			decoder := json.NewDecoder(&output)
			encoded := output.String()
			if decoder.Decode(&fields) != nil || len(fields) != 12 || fields["result"] != tc.want {
				t.Fatal("wrong diagnostic JSON outcome", encoded)
			}
			if decoder.Decode(&fields) != io.EOF {
				t.Fatal("diagnose emitted extra output")
			}
			for _, secret := range []string{path, "private-diagnostic", "PRIVATE-", "gpt-6.1-sol", "Bearer", "https://"} {
				if strings.Contains(encoded, secret) {
					t.Fatal("diagnostic command disclosed private data")
				}
			}
			if requests != tc.requests {
				t.Fatal("diagnostic refreshed/retried or skipped metadata", requests)
			}
			if tc.requests == 0 && fields["http_status"] != nil {
				t.Fatal("invented HTTP status")
			}
			if tc.missing {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("diagnose created private state")
				}
			} else {
				after, err := os.ReadFile(filepath.Join(path, "record.json"))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("diagnose changed persistent credential state")
				}
				if !tc.corrupt && !tc.locked {
					r, err := openRuntime(config.Default(), commandDeps{state: path, transport: deps.transport}, true)
					if err != nil {
						t.Fatal(err)
					}
					status := r.Manager.Status()
					r.Close()
					wantGeneration := uint64(1)
					if tc.pending {
						wantGeneration = 2
					}
					if status.Generation != wantGeneration || status.AuthEpoch != 1 {
						t.Fatal("diagnose changed lifecycle counters")
					}
				}
			}
		})
	}
}

type diagnosticFailWriter struct{}

func (diagnosticFailWriter) Write([]byte) (int, error) { return 0, errors.New("PRIVATE-WRITER-PATH") }

func TestDiagnosticWriterAndArgumentsStaySanitized(t *testing.T) {
	err := writeDiagnostic(diagnosticFailWriter{}, codex.DiagnosticReport{})
	if err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("diagnostic writer error leaked")
	}
	for _, args := range [][]string{{"auth", "diagnose", "PRIVATE-TOKEN"}, {"auth", "diagnose", "--refresh"}} {
		if err := run(context.Background(), args, io.Discard); err == nil || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("diagnostic accepted extra arguments")
		}
	}
}
