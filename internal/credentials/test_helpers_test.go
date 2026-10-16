package credentials

import (
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
)

var testNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func jwt(account string, expiry time.Time) string {
	b, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_id": account}, "exp": expiry.Unix()})
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." + base64.RawURLEncoding.EncodeToString(b) + ".c2ln"
}
func tokenJSON(account, refresh string, expiry time.Time) string {
	b, _ := json.Marshal(map[string]any{"access_token": jwt(account, expiry), "refresh_token": refresh, "expires_in": 3600})
	return string(b)
}
func activeRecord(expiry time.Time) Record {
	r := emptyRecord()
	r.State = "active"
	r.Generation = 1
	r.AuthEpoch = 1
	r.Access = jwt("account-test", expiry)
	r.Refresh = "refresh-original"
	r.Account = "account-test"
	r.IssuedAt = testNow.Add(-time.Minute)
	r.SavedAt = testNow
	r.ExpiresAt = expiry
	return r
}
func testStore(t *testing.T) (*DiskStore, string) {
	t.Helper()
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "private")
	s, e := OpenStore(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}
func seed(t *testing.T, s *DiskStore, r Record) {
	t.Helper()
	i := tombstone(r, "unusable", r.SavedAt)
	if e := s.Begin(context.Background(), i); e != nil {
		t.Fatal(e)
	}
	if e := s.Commit(context.Background(), r); e != nil {
		t.Fatal(e)
	}
}
func manager(t *testing.T, s Storage, transport http.RoundTripper, invalidate func()) *Manager {
	t.Helper()
	m, e := NewManager(s, ManagerOptions{OAuth: OAuthClient{Transport: transport, Now: func() time.Time { return testNow }}, Invalidate: invalidate})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { m.Close() })
	return m
}
