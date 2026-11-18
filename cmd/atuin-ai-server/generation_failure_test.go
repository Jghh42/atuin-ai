package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

type diagnosticLogWriter chan []byte

func (w diagnosticLogWriter) Write(b []byte) (int, error) {
	w <- append([]byte(nil), b...)
	return len(b), nil
}
func TestServeComposesPrivacySafeGenerationObserver(t *testing.T) {
	root := t.TempDir()
	if os.Chmod(root, 0700) != nil {
		t.Fatal("private fixture")
	}
	path := filepath.Join(root, "SYNTHETIC-state")
	seedDiagnosticSession(t, path, time.Now().UTC().Add(time.Hour), false)
	socket, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := socket.Addr().(*net.TCPAddr).Port
	socket.Close()
	cfg := filepath.Join(root, "SYNTHETIC-config.toml")
	if os.WriteFile(cfg, []byte(fmt.Sprintf("port = %d\n", port)), 0600) != nil {
		t.Fatal("private config")
	}
	var calls atomic.Int32
	deps := commandDeps{state: path, transport: authTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.String() != "https://chatgpt.com/backend-api/codex/responses" {
			t.Error("extra endpoint")
		}
		return &http.Response{StatusCode: 401, Header: http.Header{"SYNTHETIC-header": []string{"SYNTHETIC-secret"}}, Body: io.NopCloser(strings.NewReader("SYNTHETIC-error-body"))}, nil
	})}
	logs := make(diagnosticLogWriter, 8)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan error, 1)
	go func() { done <- runWith(ctx, []string{"serve", "--config", cfg}, logs, deps) }()
	defer func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error("serve failed")
			}
		case <-time.After(7 * time.Second):
			t.Error("serve did not join")
		}
	}()
	select {
	case line := <-logs:
		if !strings.Contains(string(line), `"msg":"listening"`) {
			t.Fatal("no listener")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("startup timeout")
	}
	token, err := credentials.ReadClientToken(path)
	if err != nil {
		t.Fatal("serve did not create a client token")
	}
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/api/cli/chat", port), strings.NewReader(`{"messages":[{"role":"user","content":"SYNTHETIC-private-prompt"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || strings.Count(string(b), "event: error") != 1 || strings.Contains(string(b), "SYNTHETIC") {
		t.Fatal("wire changed")
	}
	select {
	case line := <-logs:
		var fields map[string]any
		if json.Unmarshal(line, &fields) != nil || len(fields) != 7 || fields["msg"] != "generation_failed" || fields["category"] != "http_unauthorized" || fields["upstream_http_status"] != float64(401) {
			t.Fatal("production allowlist")
		}
		if strings.Contains(string(line), "SYNTHETIC") || strings.Contains(string(line), "private-diagnostic") {
			t.Fatal("privacy leak")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missing generation report")
	}
	if calls.Load() != 1 {
		t.Fatal("generation count changed")
	}
}
