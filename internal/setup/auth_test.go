package setup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

type syntheticStore struct {
	record         credentials.Record
	writes, closed int
}

func (s *syntheticStore) Load() (credentials.Record, error) { return s.record, nil }
func (s *syntheticStore) Begin(context.Context, credentials.Record) error {
	s.writes++
	return credentials.ErrStore
}
func (s *syntheticStore) Commit(context.Context, credentials.Record) error {
	s.writes++
	return credentials.ErrStore
}
func (s *syntheticStore) Close() error { s.closed++; return nil }

type noNetwork struct{ t *testing.T }

func (n noNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	n.t.Error("init made an unapproved network request")
	return nil, credentials.ErrUnavailable
}
func TestExpiredRefreshableRealManagerPreservedWithoutNetworkOrWrites(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	expiry := now.Add(-time.Hour)
	claims, _ := json.Marshal(map[string]any{"exp": expiry.Unix(), "https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "synthetic-account"}})
	access := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims) + ".c2ln"
	record := credentials.Record{Schema: 1, Provider: "codex", Registration: credentials.ClientID, State: "active", Generation: 7, AuthEpoch: 2, Account: "synthetic-account", Access: access, Refresh: "synthetic-refresh", IssuedAt: now.Add(-2 * time.Hour), SavedAt: now.Add(-90 * time.Minute), ExpiresAt: expiry}
	store := &syntheticStore{record: record}
	manager, e := credentials.NewManager(store, credentials.ManagerOptions{OAuth: credentials.OAuthClient{Transport: noNetwork{t}, Now: func() time.Time { return now }}})
	if e != nil {
		t.Fatal(e)
	}
	w, _, _, events := fixture("n", "n")
	w.OpenAuth = func() (Auth, error) { return manager, nil }
	if e = w.Run(context.Background()); e != nil {
		t.Fatal(e)
	}
	if store.writes != 0 || store.closed != 1 || store.record != record || hasEvent(*events, "probe") {
		t.Fatal("saved session changed")
	}
}
