package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/codexproof"
)

func TestExplicitApprovalAndPrivateDisplayRequired(t *testing.T) {
	for _, args := range [][]string{nil, {"login"}, {"prove"}, {"login", "--approve-live"}, {"login", "--approve-live", "--host", "0.0.0.0"}, {"unknown", "--approve-live"}} {
		var out bytes.Buffer
		if code := run(context.Background(), args, &out); code != 2 {
			t.Fatalf("accepted unsafe args %v exit=%d", args, code)
		}
	}
}

func TestNonTerminalCharacterDeviceCannotReceiveCodes(t *testing.T) {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if code := run(context.Background(), []string{"login", "--approve-live"}, f); code != 2 {
		t.Fatal("non-terminal character device accepted")
	}
}

func TestDeviceDisplayPoliciesAndExpiredCode(t *testing.T) {
	now := time.Now()
	p := &devicePage{now: func() time.Time { return now }, prompt: &codexproof.DevicePrompt{UserCode: "SYNTHETIC-PRIVATE", VerificationURI: "https://auth.openai.com/codex/device", ExpiresAt: now.Add(time.Minute)}}
	for _, tc := range []struct {
		method, host, path string
		status             int
	}{
		{"GET", "127.0.0.1:1455", "/", http.StatusOK},
		{"GET", "evil.invalid", "/", http.StatusBadRequest},
		{"GET", "localhost:1455", "/", http.StatusBadRequest},
		{"GET", "127.0.0.1:1455", "/?query=1", http.StatusBadRequest},
		{"GET", "127.0.0.1:1455", "/auth/callback", http.StatusBadRequest},
		{"POST", "127.0.0.1:1455", "/", http.StatusBadRequest},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = tc.host
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s got%d", tc.method, tc.path, w.Code)
		}
		for _, header := range []string{"Cache-Control", "Referrer-Policy", "Content-Security-Policy", "X-Content-Type-Options"} {
			if w.Header().Get(header) == "" {
				t.Fatal("missing browser policy")
			}
		}
		if tc.status != 200 && strings.Contains(w.Body.String(), "SYNTHETIC-PRIVATE") {
			t.Fatal("private code leaked to rejected request")
		}
	}
	now = now.Add(2 * time.Minute)
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "127.0.0.1:1455"
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusGone || strings.Contains(w.Body.String(), "SYNTHETIC-PRIVATE") {
		t.Fatal("expired code exposed")
	}
}
