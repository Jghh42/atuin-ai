package application

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

type diagnosticRoundTrip func(*http.Request) (*http.Response, error)

func (f diagnosticRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func seedApplicationDiagnostic(t *testing.T, path string, now time.Time) {
	t.Helper()
	r := credentials.Record{Schema: 1, Provider: "codex", Registration: credentials.ClientID, State: "unusable", Generation: 1, AuthEpoch: 1, SavedAt: now}
	s, err := credentials.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Begin(context.Background(), r) != nil {
		t.Fatal("diagnostic seed intent failed")
	}
	r.State, r.Access, r.Refresh, r.Account = "active", syntheticJWT(now), "fake-diagnostic-refresh", "test-account-private"
	r.IssuedAt, r.ExpiresAt = now.Add(-time.Minute), now.Add(time.Hour)
	if s.Commit(context.Background(), r) != nil {
		t.Fatal("diagnostic seed record failed")
	}
}
func TestDiagnosticCompositionNoRefreshAndNoCacheMutation(t *testing.T) {
	now := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	path := privatePath(t)
	seedApplicationDiagnostic(t, path, now)
	var requests atomic.Int32
	transport := diagnosticRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.URL.String() != codex.ModelsURL || r.Header.Get("ChatGPT-Account-Id") != "test-account-private" || r.Header.Get("Authorization") != "Bearer "+syntheticJWT(now) {
			t.Error("diagnostic composition lost native identity binding")
		}
		return response(200, `{"models":[{"slug":"gpt-6.1-sol","display_name":"PRIVATE-NAME","visibility":"list","priority":0}]}`), nil
	})
	r, err := newWithOptions(path, config.Default(), Options{Transport: transport, Now: func() time.Time { return now }}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	status := r.Manager.Status()
	if report := r.Diagnose(context.Background()); report.Result != codex.ResultSuccess || requests.Load() != 1 {
		t.Fatal("diagnostic failed")
	}
	if _, err := r.Catalog.Get(context.Background()); err != nil || requests.Load() != 1 {
		t.Fatal("ordinary catalog made a remote request")
	}
	if report := r.Diagnose(context.Background()); report.Result != codex.ResultSuccess || requests.Load() != 2 {
		t.Fatal("diagnostic reused cached data")
	}
	if _, err := r.Catalog.Get(context.Background()); err != nil || requests.Load() != 2 {
		t.Fatal("diagnostic invalidated ordinary cache")
	}
	if status != r.Manager.Status() {
		t.Fatal("diagnostic changed session")
	}
	// Expiry-minus-skew must NOT invoke Current/refresh or send metadata.
	now = now.Add(time.Hour - credentials.AccessSkew)
	if report := r.Diagnose(context.Background()); report.Result != codex.ResultRefreshRequired || report.HTTPStatus != nil || requests.Load() != 2 {
		t.Fatal("diagnostic refreshed expired access")
	}
}

func TestDiagnosticCloseInvalidatesReportAndCancelsNativeRequest(t *testing.T) {
	now := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	path := privatePath(t)
	seedApplicationDiagnostic(t, path, now)
	entered := make(chan struct{})
	transport := diagnosticRoundTrip(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	r, err := newWithOptions(path, config.Default(), Options{Transport: transport, Now: func() time.Time { return now }}, true)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan codex.DiagnosticReport, 1)
	go func() { done <- r.Diagnose(context.Background()) }()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- r.Close() }()
	select {
	case report := <-done:
		if report.Result != codex.ResultLifecycleInvalidated || report.HTTPStatus != nil || report.ModelCount != nil || report.ProvenModelPresent != nil {
			t.Fatal("stale diagnostic report survived Close")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close failed to cancel native metadata")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close failed to join diagnostic")
	}
}
