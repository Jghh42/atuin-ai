package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/config"
)

func TestServerRequiresValidClientToken(t *testing.T) {
	for _, token := range []string{"", "secret", testToken + "0", strings.ToUpper(testToken), "aia1_" + strings.Repeat("g", 64)} {
		if _, err := newServer(config.Default(), nil, token, http.NotFoundHandler()); !errors.Is(err, ErrClientToken) {
			t.Fatalf("token %q: err=%v", token, err)
		}
	}
}

func TestClientAuthenticationBeforeRouting(t *testing.T) {
	entered := 0
	s, err := newServer(config.Default(), nil, testToken, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered++
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}
	const body = `{"messages":[{"role":"user","content":"hi"}]}`
	cases := []struct {
		name, method, path, host string
		auth                     []string
		origin                   string
		status                   int
		code                     string
	}{
		{"valid", "POST", "/api/cli/chat", "127.0.0.1:17391", []string{"Bearer " + testToken}, "", 204, ""},
		{"localhost", "POST", "/api/cli/chat", "localhost:17391", []string{"Bearer " + testToken}, "", 204, ""},
		{"missing", "POST", "/api/cli/chat", "127.0.0.1:17391", nil, "", 401, "unauthorized"},
		{"wrong token", "POST", "/api/cli/chat", "127.0.0.1:17391", []string{"Bearer aia1_" + strings.Repeat("0", 64)}, "", 401, "unauthorized"},
		{"bare token", "POST", "/api/cli/chat", "127.0.0.1:17391", []string{testToken}, "", 401, "unauthorized"},
		{"other scheme", "POST", "/api/cli/chat", "127.0.0.1:17391", []string{"Basic " + testToken}, "", 401, "unauthorized"},
		{"duplicate", "POST", "/api/cli/chat", "127.0.0.1:17391", []string{"Bearer " + testToken, "Bearer " + testToken}, "", 401, "unauthorized"},
		{"models unauthenticated", "GET", "/api/cli/models", "127.0.0.1:17391", nil, "", 401, "unauthorized"},
		{"unknown route unauthenticated", "GET", "/nope", "127.0.0.1:17391", nil, "", 401, "unauthorized"},
		{"unknown route authenticated", "GET", "/nope", "127.0.0.1:17391", []string{"Bearer " + testToken}, "", 404, "not_found"},
		// A hostile page reaching 127.0.0.1 directly cannot attach the token.
		{"cross-site simple request", "POST", "/api/cli/chat", "127.0.0.1:17391", nil, "https://attacker.example", 401, "unauthorized"},
		// After DNS rebinding the Host names the attacker's domain.
		{"rebinding host", "POST", "/api/cli/chat", "rebind.attacker.example:17391", []string{"Bearer " + testToken}, "https://attacker.example", 403, "forbidden_host"},
		{"empty host", "POST", "/api/cli/chat", "", []string{"Bearer " + testToken}, "", 403, "forbidden_host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := entered
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
			r.Host = tc.host
			for _, v := range tc.auth {
				r.Header.Add("Authorization", v)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
				r.Header.Set("Content-Type", "text/plain;charset=UTF-8")
			}
			w := httptest.NewRecorder()
			s.http.Handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want %d", w.Code, tc.status)
			}
			reached := entered != before
			if reached != (tc.status == 204) {
				t.Fatalf("chat handler reached=%v", reached)
			}
			if tc.code == "" {
				return
			}
			var got struct{ Error, Message string }
			if json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Error != tc.code {
				t.Fatalf("body=%q", w.Body.String())
			}
			if strings.Contains(w.Body.String(), testToken) {
				t.Fatal("error echoed the client token")
			}
			if tc.status == 401 && w.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing bearer challenge")
			}
		})
	}
}
