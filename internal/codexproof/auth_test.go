package codexproof

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testHome(t *testing.T) { t.Helper(); t.Setenv("HOME", privateTempDir(t)) }
func jwtToken(account string, exp int64) string {
	b, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": account}, "exp": exp})
	return "e30." + base64.RawURLEncoding.EncodeToString(b) + ".c2ln"
}
func fakeClient(t *testing.T, h http.HandlerFunc) *authClient {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c := newClient()
	c.base = s.URL
	return c
}
func tokenJSON(account, refresh string) string {
	b, _ := json.Marshal(map[string]any{"access_token": jwtToken(account, time.Now().Add(time.Hour).Unix()), "refresh_token": refresh, "expires_in": 3600})
	return string(b)
}
func TestDeviceFlowProjectionAndPolling(t *testing.T) {
	testHome(t)
	var polls int
	var waits []time.Duration
	c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("wrong method")
		}
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			var v map[string]string
			_ = json.NewDecoder(r.Body).Decode(&v)
			if len(v) != 1 || v["client_id"] != clientID || r.Header.Get("Content-Type") != "application/json" {
				t.Error("wrong start projection")
			}
			fmt.Fprint(w, `{"device_auth_id":"device-secret","user_code":"user-secret","interval":"2"}`)
		case "/api/accounts/deviceauth/token":
			var v map[string]string
			_ = json.NewDecoder(r.Body).Decode(&v)
			if len(v) != 2 || v["device_auth_id"] != "device-secret" || v["user_code"] != "user-secret" {
				t.Error("wrong poll projection")
			}
			polls++
			switch polls {
			case 1:
				w.WriteHeader(403)
				fmt.Fprint(w, `secret`)
			case 2:
				w.WriteHeader(404)
			case 3:
				w.WriteHeader(400)
				fmt.Fprint(w, `{"error":{"code":"deviceauth_authorization_pending"}}`)
			case 4:
				w.WriteHeader(429)
				fmt.Fprint(w, `{"error":"slow_down"}`)
			default:
				fmt.Fprint(w, `{"authorization_code":"code-secret","code_verifier":"verifier-secret"}`)
			}
		case "/oauth/token":
			_ = r.ParseForm()
			v := r.PostForm
			if len(v) != 5 || v.Get("grant_type") != "authorization_code" || v.Get("client_id") != clientID || v.Get("code") != "code-secret" || v.Get("code_verifier") != "verifier-secret" || v.Get("redirect_uri") != deviceRedirectURI || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Error("wrong exchange projection")
			}
			fmt.Fprint(w, tokenJSON("account", "refresh-secret"))
		default:
			t.Error("unexpected request")
			w.WriteHeader(500)
		}
	})
	c.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	displayed := false
	e := login(context.Background(), func(p DevicePrompt) error {
		displayed = true
		if p.UserCode != "user-secret" || p.VerificationURI != verificationURI || time.Until(p.ExpiresAt) < 14*time.Minute {
			t.Error("bad prompt")
		}
		if strings.Contains(fmt.Sprintf("%v %#v", p, p), "user-secret") {
			t.Error("prompt leaked")
		}
		return nil
	}, c)
	if e != nil || !displayed || polls != 5 || fmt.Sprint(waits) != "[2s 2s 2s 7s]" {
		t.Fatalf("flow failed: %v %v", e, waits)
	}
	st, e := Status()
	if e != nil || !st.LoggedIn || st.Generation != 1 {
		t.Fatal("bad status")
	}
	_ = WithRecord(context.Background(), func(r Record) error {
		if r.AccountID != "account" || r.RefreshToken != "refresh-secret" {
			t.Error("bad saved record")
		}
		return nil
	})
	// Existing renewable sessions must never be replaced, even with expired access.
	if login(context.Background(), func(DevicePrompt) error { t.Error("display called"); return nil }, c) == nil {
		t.Fatal("overwrote active session")
	}
}
func TestDeviceCancellationExpiryAndSafeErrors(t *testing.T) {
	for _, mode := range []string{"cancel", "expiry", "failure", "oversize", "display"} {
		t.Run(mode, func(t *testing.T) {
			testHome(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "usercode") {
					fmt.Fprint(w, `{"device_auth_id":"private-id","user_code":"private-code","interval":0}`)
					return
				}
				if mode == "failure" {
					w.WriteHeader(500)
					fmt.Fprint(w, "private-secret")
					return
				}
				if mode == "oversize" {
					fmt.Fprint(w, strings.Repeat("private-secret", maxBody))
					return
				}
				w.WriteHeader(404)
			})
			now := time.Now()
			c.now = func() time.Time { return now }
			c.sleep = func(_ context.Context, d time.Duration) error {
				if mode == "cancel" {
					cancel()
					return errSafe
				}
				now = now.Add(16 * time.Minute)
				return nil
			}
			e := login(ctx, func(DevicePrompt) error {
				if mode == "display" {
					return fmt.Errorf("private-secret")
				}
				return nil
			}, c)
			if e == nil || strings.Contains(e.Error(), "private") {
				t.Fatal("unsafe result")
			}
			st, e := Status()
			if e != nil || st.LoggedIn || st.Generation != 0 {
				t.Fatal("failed login changed credentials")
			}
		})
	}
}
func TestExchangeValidation(t *testing.T) {
	valid := map[string]any{"access_token": jwtToken("account", time.Now().Add(time.Hour).Unix()), "refresh_token": "new-refresh", "expires_in": 3600}
	for _, field := range []string{"access_token", "refresh_token", "expires_in", "account", "exp", "rotated", "binding"} {
		t.Run(field, func(t *testing.T) {
			v := map[string]any{}
			for k, x := range valid {
				v[k] = x
			}
			old := Record{}
			switch field {
			case "account":
				v["access_token"] = jwtToken("", time.Now().Add(time.Hour).Unix())
			case "exp":
				v["access_token"] = jwtToken("account", time.Now().Add(-time.Hour).Unix())
			case "rotated":
				old.RefreshToken = "new-refresh"
			case "binding":
				old.AccountID = "other"
			default:
				delete(v, field)
			}
			c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(v) })
			if _, e := c.exchange(context.Background(), nil, old); e == nil {
				t.Fatal("invalid exchange accepted")
			}
		})
	}
	for _, raw := range []string{"bad", "e30.e30.c2ln", "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":7}}`)) + ".c2ln"} {
		if _, _, e := accountFromJWT(raw, time.Now()); e == nil {
			t.Fatal("invalid JWT metadata accepted")
		}
	}
}
func TestRefreshRotationLockAndLocalLogout(t *testing.T) {
	testHome(t)
	s, e := openStore()
	if e != nil {
		t.Fatal(e)
	}
	_ = s.save(Record{AccessToken: "old-access", RefreshToken: "old-refresh", AccountID: "account", Generation: 4})
	s.close()
	calls := 0
	c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = r.ParseForm()
		if r.PostForm.Get("refresh_token") != "old-refresh" || r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("client_id") != clientID || len(r.PostForm) != 3 {
			t.Error("bad refresh projection")
		}
		if e := WithRecord(context.Background(), func(Record) error { return nil }); e == nil {
			t.Error("network operation unlocked")
		}
		fmt.Fprint(w, tokenJSON("account", "new-refresh"))
	})
	if e := refresh(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	_ = WithRecord(context.Background(), func(r Record) error {
		if r.RefreshToken != "new-refresh" || r.Generation != 5 {
			t.Error("rotation not persisted")
		}
		if e := Logout(context.Background()); e == nil {
			t.Error("callback lock not held")
		}
		if strings.Contains(fmt.Sprintf("%v %#v", r, r), "refresh") {
			t.Error("record leaked")
		}
		return nil
	})
	if e := Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	st, e := Status()
	if e != nil || st.LoggedIn || st.Generation != 6 || !st.ExpiresAt.IsZero() || calls != 1 {
		t.Fatal("bad local logout")
	}
	b, e := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".atuin-ai-codex-proof", "record.json"))
	if e != nil || strings.Contains(string(b), "refresh") {
		t.Fatal("retained secrets")
	}
}
func TestIntervalValidation(t *testing.T) {
	for _, raw := range []string{`0`, `"2"`, `2.5`} {
		if _, e := parseInterval(json.RawMessage(raw)); e != nil {
			t.Fatal("valid interval rejected")
		}
	}
	for _, raw := range []string{`null`, `-1`, `"NaN"`, `"Infinity"`, `901`, `{}`} {
		if _, e := parseInterval(json.RawMessage(raw)); e == nil {
			t.Fatal("invalid interval accepted")
		}
	}
}
func TestNoRedirect(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	status, _, e := c.post(context.Background(), "/oauth/token", "application/json", "{}")
	if e != nil || status != http.StatusTemporaryRedirect || reached {
		t.Fatal("redirect followed")
	}
}
