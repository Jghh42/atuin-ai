package httpserver

import (
	"context"
	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnwiredProductionFailsClosed(t *testing.T) {
	s, err := New(config.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/cli/models", "/api/cli/chat"} {
		method := "GET"
		if strings.HasSuffix(path, "chat") {
			method = "POST"
		}
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`)))
		if w.Code != 503 || strings.Contains(w.Body.String(), "gpt-") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

type changingCatalog struct {
	calls     int
	snapshots []catalog.Snapshot
}

func (c *changingCatalog) Get(context.Context) (catalog.Snapshot, error) {
	c.calls++
	if c.calls > len(c.snapshots) {
		return catalog.Snapshot{}, catalog.ErrUnavailable
	}
	return c.snapshots[c.calls-1], nil
}

func TestOneCatalogReadPerRequestAndObservedDecoderResolution(t *testing.T) {
	source := &changingCatalog{}
	for _, name := range []string{"First snapshot", "Second snapshot"} {
		snapshot, err := catalog.Build([]catalog.Entry{{ID: "gpt-6.1-sol", Name: name, Visibility: "list"}}, "")
		if err != nil {
			t.Fatal(err)
		}
		source.snapshots = append(source.snapshots, snapshot)
	}
	observed := 0
	s, err := newWithCatalog(config.Default(), nil, source, func(r atuin.Request) {
		observed++
		if source.calls != 2 || r.Config.ModelAlias != "gpt-6.1-sol" || r.Config.ProviderModel != "gpt-6.1-sol" {
			t.Fatalf("inconsistent decoder snapshot: calls=%d config=%+v", source.calls, r.Config)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cli/models", nil))
	if w.Code != 200 || source.calls != 1 || !strings.Contains(w.Body.String(), "First snapshot") {
		t.Fatal(w.Code, source.calls, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cli/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"config":{"model":"unknown"}}`)))
	if w.Code != 503 || source.calls != 2 || observed != 1 {
		t.Fatal(w.Code, source.calls, observed, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cli/models", nil))
	if w.Code != 503 || source.calls != 3 {
		t.Fatal(w.Code, source.calls)
	}
}

type snapshotSource struct{ snapshot catalog.Snapshot }

func (s snapshotSource) Get(context.Context) (catalog.Snapshot, error) { return s.snapshot, nil }
func TestSnapshotResolutionAndSanitizedProjection(t *testing.T) {
	snapshot, err := catalog.Build([]catalog.Entry{{ID: "gpt-6.1-sol", Name: "Sol", Visibility: "list"}, {ID: "unproven", Name: "Unproven", Visibility: "list"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewWithCatalog(config.Default(), nil, snapshotSource{snapshot})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cli/models", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "unproven") || !strings.Contains(w.Body.String(), `"alias":"gpt-6.1-sol"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cli/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}],"config":{"model":"missing"}}`)))
	if w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
}
