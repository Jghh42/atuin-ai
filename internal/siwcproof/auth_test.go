package siwcproof

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestCallbackAttacksAndOneUse(t *testing.T) {
	h := &browserHandler{host: "127.0.0.1:1455", state: "expected-secret", authorize: "https://example.invalid/authorize?secret=hidden", result: make(chan callbackResult, 1)}
	for _, tc := range []struct{ method, host, path string }{
		{"POST", h.host, "/auth/callback?state=expected-secret&code=secret&client_id=issued"},
		{"GET", "localhost:1455", "/"},
		{"GET", "evil.invalid", "/auth/callback?state=expected-secret&code=secret&client_id=issued"},
		{"GET", h.host, "/auth/callback?state=wrong&code=secret&client_id=issued"},
		{"GET", h.host, "/auth/callback?state=expected-secret&state=expected-secret&code=secret&client_id=issued"},
		{"GET", h.host, "/auth/callback?state=expected-secret&code=secret&code=other&client_id=issued"},
		{"GET", h.host, "/other?state=expected-secret&code=secret&client_id=issued"},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = tc.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 || strings.Contains(w.Body.String(), "secret") {
			t.Fatal("unsafe callback handling")
		}
		if h.used {
			t.Fatal("invalid callback consumed state")
		}
	}
	valid := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/auth/callback?state=expected-secret&code=private-code&client_id=issued", nil)
		r.Host = h.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := valid(); w.Code != 200 || strings.Contains(w.Body.String(), "private-code") {
		t.Fatal("callback failed")
	}
	if valid().Code != 400 {
		t.Fatal("replay accepted")
	}
	cb := <-h.result
	if cb.code != "private-code" || cb.client != "issued" {
		t.Fatal("callback lost")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = h.host
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	for _, header := range []string{"Cache-Control", "Referrer-Policy", "Content-Security-Policy"} {
		if w.Header().Get(header) == "" {
			t.Fatal("missing browser policy")
		}
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("unexpected cookie")
	}
}
func TestDynamicClientRejected(t *testing.T) {
	for _, client := range []string{"", dynamicClient} {
		h := &browserHandler{host: "127.0.0.1:1455", state: "s", result: make(chan callbackResult, 1)}
		r := httptest.NewRequest("GET", "/auth/callback?state=s&code=c&client_id="+client, nil)
		r.Host = h.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 || !h.used {
			t.Fatal("unissued client accepted")
		}
	}
}

func TestJWTValidationAndRotation(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal("key generation")
	}
	var checkForm func(url.Values)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
		case "/token":
			if r.Method != "POST" || r.ParseForm() != nil {
				t.Error("invalid token request")
			}
			if checkForm != nil {
				checkForm(r.Form)
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "replacement-access", "refresh_token": "replacement-refresh", "token_type": "Bearer", "scope": "openid offline_access resource.invoke chatgpt.tokens.use.direct", "expires_in": 3600, "earliest_refresh_at": time.Now().Add(time.Minute).Format(time.RFC3339)})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := &authClient{http: srv.Client(), issuer: "https://issuer.invalid"}
	d := discovery{JWKS: srv.URL + "/jwks", Token: srv.URL + "/token"}
	now := time.Now()
	base := idClaims{RegisteredClaims: jwt.RegisteredClaims{Issuer: c.issuer, Subject: "subject", Audience: jwt.ClaimStrings{"issued"}, IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}, Nonce: "nonce"}
	sign := func(cl idClaims, method jwt.SigningMethod, k any) string {
		tok := jwt.NewWithClaims(method, &cl)
		tok.Header["kid"] = "test"
		s, e := tok.SignedString(k)
		if e != nil {
			t.Fatal("signing failed")
		}
		return s
	}
	if sub, e := c.verify(context.Background(), d, sign(base, jwt.SigningMethodRS256, key), "issued", "nonce", ""); e != nil || sub != "subject" {
		t.Fatal("valid token rejected")
	}
	for _, mut := range []func(*idClaims){func(c *idClaims) { c.Issuer = "wrong" }, func(c *idClaims) { c.Audience = jwt.ClaimStrings{"other"} }, func(c *idClaims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Minute)) }, func(c *idClaims) { c.IssuedAt = nil }, func(c *idClaims) { c.IssuedAt = jwt.NewNumericDate(now.Add(time.Hour)) }, func(c *idClaims) { c.Nonce = "wrong" }, func(c *idClaims) { c.Subject = "" }} {
		cl := base
		mut(&cl)
		if _, e := c.verify(context.Background(), d, sign(cl, jwt.SigningMethodRS256, key), "issued", "nonce", ""); e == nil {
			t.Fatal("invalid JWT accepted")
		}
	}
	for _, mutate := range []func(*idClaims){
		func(cl *idClaims) { cl.AuthorizedParty = "other" },
		func(cl *idClaims) { cl.Audience = jwt.ClaimStrings{"issued", "other"} },
		func(cl *idClaims) { cl.ExpiresAt = nil },
	} {
		cl := base
		mutate(&cl)
		if _, e := c.verify(context.Background(), d, sign(cl, jwt.SigningMethodRS256, key), "issued", "nonce", ""); e == nil {
			t.Fatal("invalid authorized party, audience or expiry accepted")
		}
	}
	if _, e := c.verify(context.Background(), d, sign(base, jwt.SigningMethodHS256, []byte("wrong")), "issued", "nonce", ""); e == nil {
		t.Fatal("algorithm confusion")
	}
	if _, e := c.verify(context.Background(), d, sign(base, jwt.SigningMethodRS256, key), "issued", "", "other-subject"); e == nil {
		t.Fatal("identity substitution")
	}
	checkForm = func(f url.Values) {
		if f.Get("refresh_token") != "old-refresh" || f.Get("client_id") != "issued" || f.Get("resource") != resource {
			t.Error("bad refresh form")
		}
	}
	r, e := c.exchange(context.Background(), d, url.Values{"grant_type": {"refresh_token"}, "client_id": {"issued"}, "refresh_token": {"old-refresh"}, "resource": {resource}}, Record{ClientID: "issued", RefreshToken: "old-refresh", Generation: 3, Subject: "subject"}, "", false)
	if e != nil || r.Generation != 4 || r.RefreshToken != "replacement-refresh" || r.AccessToken != "replacement-access" || r.EarliestRefreshAt.IsZero() {
		t.Fatal("rotation failed")
	}
	s, e := openStoreAt(filepath.Join(privateTempDir(t), "private"))
	if e != nil {
		t.Fatal("store failed")
	}
	defer s.close()
	if s.save(r) != nil {
		t.Fatal("persist failed")
	}
	got, e := s.load()
	if e != nil || got.Generation != 4 || got.RefreshToken != r.RefreshToken || got.AccessToken != r.AccessToken {
		t.Fatal("rotation not atomic record")
	}
}

func TestRejectMissingGrantOrReplacementRefresh(t *testing.T) {
	for _, kind := range []string{"missing_initial_scope", "missing_direct_scope", "missing_replacement", "same_refresh"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				token := map[string]any{"access_token": "new-access", "refresh_token": "new-refresh", "token_type": "Bearer", "expires_in": 3600, "scope": "openid offline_access resource.invoke chatgpt.tokens.use.direct", "id_token": "untrusted-not-validated"}
				if kind != "missing_initial_scope" {
					delete(token, "id_token")
				}
				switch kind {
				case "missing_initial_scope":
					delete(token, "scope")
				case "missing_direct_scope":
					token["scope"] = "openid offline_access resource.invoke"
				case "missing_replacement":
					delete(token, "refresh_token")
				case "same_refresh":
					token["refresh_token"] = "old-refresh"
				}
				json.NewEncoder(w).Encode(token)
			}))
			defer server.Close()
			c := &authClient{http: server.Client()}
			_, err := c.exchange(context.Background(), discovery{Token: server.URL}, url.Values{}, Record{ClientID: "issued", RefreshToken: "old-refresh", Scopes: []string{"openid", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"}}, "nonce", kind == "missing_initial_scope")
			if err == nil {
				t.Fatal("invalid token grant accepted")
			}
		})
	}
}

func TestPKCEAndBounds(t *testing.T) {
	v, e := randomString()
	if e != nil || len(v) != 43 {
		t.Fatal("invalid verifier")
	}
	sum := sha256.Sum256([]byte(v))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if len(challenge) != 43 || challenge == v {
		t.Fatal("invalid challenge")
	}
	if _, e := readLimited(strings.NewReader(strings.Repeat("x", maxBody+1))); e == nil {
		t.Fatal("oversized body accepted")
	}
	for _, raw := range []string{`"2026-10-16T00:00:00Z"`, `1792108800`, `"1792108800"`} {
		if tm, e := parseEarliest(json.RawMessage(raw)); e != nil || tm.IsZero() {
			t.Fatal("refresh timestamp rejected")
		}
	}
	if _, e := parseEarliest(json.RawMessage(`"secret-invalid"`)); e == nil || e.Error() != errSafe.Error() {
		t.Fatal("unsafe timestamp error")
	}
}

func TestHTTPRedirectAndErrors(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			hit = true
		}
		http.Redirect(w, r, "/target?secret=never", http.StatusFound)
	}))
	defer server.Close()
	c := newClient()
	if e := c.request(context.Background(), "GET", server.URL, nil, nil); e == nil || e.Error() != errSafe.Error() || hit {
		t.Fatal("redirect followed or unsafe error")
	}
}
