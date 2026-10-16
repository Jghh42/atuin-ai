package codex

import (
	"context"
	"errors"
	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func credential() credentials.Credential {
	return credentials.Credential{Identity: credentials.Identity{Provider: "codex", Account: "fake-account", Epoch: 1}, AccessToken: "fake-token"}
}

const valid = `{"models":[{"slug":"gpt-6.1-sol","display_name":"Sol","description":null,"visibility":"list","priority":2,"unknown_additive":true}]}`

func TestNativeContract(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/backend-api/codex/models" || r.URL.Query().Get("client_version") != "0.134.0" || r.Header.Get("Authorization") != "Bearer fake-token" || r.Header.Get("ChatGPT-Account-Id") != "fake-account" {
			t.Error("wrong native discovery request")
		}
		_, _ = io.WriteString(w, valid)
	}))
	defer s.Close()
	d := Discovery{URL: s.URL + "/backend-api/codex/models?client_version=0.134.0"}
	c := catalog.New(credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return credential(), nil }), d, "", nil)
	got, err := c.Get(context.Background())
	if err != nil || got.Resolve("unknown") != "gpt-6.1-sol" {
		t.Fatal(got, err)
	}
}
func TestMalformedBoundedAndStatus(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		want   error
	}{
		{valid, 200, nil}, {`{`, 200, catalog.ErrInvalid}, {`{"models":[]}`, 200, catalog.ErrInvalid}, {`{"models":[{}]}`, 200, catalog.ErrInvalid}, {strings.Replace(valid, `"priority":2`, `"priority":2.5`, 1), 200, catalog.ErrInvalid}, {strings.Replace(valid, `"priority":2`, `"priority":2147483648`, 1), 200, catalog.ErrInvalid}, {strings.Replace(valid, `"priority":2`, `"priority":null`, 1), 200, catalog.ErrInvalid}, {strings.Repeat("x", MaxCatalogBytes+1), 200, catalog.ErrInvalid}, {"", 401, catalog.ErrUnauthorized}, {"", 403, catalog.ErrUnauthorized}, {"", 429, catalog.ErrTransient}, {"", 500, catalog.ErrTransient}, {"", 404, catalog.ErrInvalid}, {"", 302, catalog.ErrInvalid},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://invalid.example/")
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		}))
		_, err := (Discovery{URL: s.URL}).Fetch(context.Background(), credential())
		s.Close()
		if !errors.Is(err, tc.want) {
			t.Fatal(tc.status, err)
		}
	}
}
func TestCountBound(t *testing.T) {
	model := `{"slug":"gpt-6.1-sol","display_name":"Sol","visibility":"list","priority":0}`
	body := `{"models":[` + strings.Repeat(model+",", 256) + model + `]}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
	defer s.Close()
	if _, err := (Discovery{URL: s.URL}).Fetch(context.Background(), credential()); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestCancellation(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(stopped) }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := (Discovery{URL: s.URL}).Fetch(ctx, credential())
	if !errors.Is(err, catalog.ErrTransient) {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("HTTP not cancelled")
	}
}
