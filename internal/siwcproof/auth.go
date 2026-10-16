package siwcproof

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "https://auth.openai.com"
const resource = "https://api.openai.com/v1"
const dynamicClient = "dynamic_agent_client"
const maxBody = 1 << 20

func readLimited(r io.Reader) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, maxBody+1))
	if e != nil || len(b) > maxBody {
		return nil, errSafe
	}
	return b, nil
}
func randomString() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", errSafe
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func hostID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", errSafe
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func secureEqual(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

type discovery struct {
	Issuer        string `json:"issuer"`
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	Revoke        string `json:"revocation_endpoint"`
	JWKS          string `json:"jwks_uri"`
}
type authClient struct {
	http   *http.Client
	issuer string
}

func newClient() *authClient {
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, DisableKeepAlives: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 32768}
	return &authClient{&http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, issuer}
}
func (c *authClient) request(ctx context.Context, method, endpoint string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, e := http.NewRequestWithContext(ctx, method, endpoint, body)
	if e != nil {
		return errSafe
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")
	resp, e := c.http.Do(req)
	if e != nil {
		return errSafe
	}
	defer resp.Body.Close()
	b, e := readLimited(resp.Body)
	if e != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errSafe
	}
	if out != nil && json.Unmarshal(b, out) != nil {
		return errSafe
	}
	return nil
}
func (c *authClient) discover(ctx context.Context) (discovery, error) {
	var d discovery
	if c.request(ctx, "GET", c.issuer+"/.well-known/openid-configuration", nil, &d) != nil || d.Issuer != c.issuer {
		return d, errSafe
	}
	for _, s := range []string{d.Authorization, d.Token, d.JWKS, d.Revoke} {
		u, e := url.Parse(s)
		if e != nil || u.Scheme != "https" || u.Host != "auth.openai.com" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
			return d, errSafe
		}
	}
	if d.Authorization != issuer+"/api/accounts/authorize" {
		return d, errSafe
	}
	return d, nil
}

type idClaims struct {
	jwt.RegisteredClaims
	Nonce           string `json:"nonce"`
	AuthorizedParty string `json:"azp"`
}

func (c *authClient) verify(ctx context.Context, d discovery, raw, client, nonce, subject string) (string, error) {
	var keys struct {
		Keys []struct{ Kty, Kid, Use, Alg, N, E string }
	}
	if c.request(ctx, "GET", d.JWKS, nil, &keys) != nil {
		return "", errSafe
	}
	claims := &idClaims{}
	_, e := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errSafe
		}
		var key *rsa.PublicKey
		for _, k := range keys.Keys {
			if k.Kid != kid || k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") || (k.Alg != "" && k.Alg != "RS256") {
				continue
			}
			if key != nil {
				return nil, errSafe
			}
			n, e := base64.RawURLEncoding.DecodeString(k.N)
			if e != nil || len(n) < 256 || len(n) > 1024 {
				return nil, errSafe
			}
			b, e := base64.RawURLEncoding.DecodeString(k.E)
			if e != nil || len(b) == 0 || len(b) > 4 {
				return nil, errSafe
			}
			exp := new(big.Int).SetBytes(b).Int64()
			if exp < 3 || exp > 2147483647 || exp%2 == 0 {
				return nil, errSafe
			}
			key = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exp)}
		}
		if key == nil {
			return nil, errSafe
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(c.issuer), jwt.WithAudience(client), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if e != nil || len(claims.Audience) != 1 || (claims.AuthorizedParty != "" && claims.AuthorizedParty != client) || claims.IssuedAt == nil || claims.IssuedAt.Time.After(time.Now()) || claims.Subject == "" || (nonce != "" && !secureEqual(claims.Nonce, nonce)) || (subject != "" && !secureEqual(claims.Subject, subject)) {
		return "", errSafe
	}
	return claims.Subject, nil
}

type tokenResponse struct {
	Access   string          `json:"access_token"`
	Refresh  string          `json:"refresh_token"`
	ID       string          `json:"id_token"`
	Type     string          `json:"token_type"`
	Scope    string          `json:"scope"`
	Expires  json.Number     `json:"expires_in"`
	Earliest json.RawMessage `json:"earliest_refresh_at"`
}

func parseEarliest(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if t, e := time.Parse(time.RFC3339, s); e == nil {
			return t, nil
		}
	} else {
		s = string(raw)
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 0 {
		return time.Time{}, errSafe
	}
	return time.Unix(n, 0), nil
}
func (c *authClient) exchange(ctx context.Context, d discovery, form url.Values, r Record, nonce string, initial bool) (Record, error) {
	var t tokenResponse
	if c.request(ctx, "POST", d.Token, form, &t) != nil {
		return Record{}, errSafe
	}
	seconds, e := t.Expires.Int64()
	if e != nil || seconds <= 0 || seconds > 31536000 || t.Access == "" || !strings.EqualFold(t.Type, "Bearer") || t.Refresh == "" || (!initial && t.Refresh == r.RefreshToken) || (initial && t.ID == "") {
		return Record{}, errSafe
	}
	scopes := strings.Fields(t.Scope)
	if !initial && t.Scope == "" {
		scopes = r.Scopes
	}
	for _, required := range []string{"openid", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"} {
		found := false
		for _, scope := range scopes {
			if scope == required {
				found = true
			}
		}
		if !found {
			return Record{}, errSafe
		}
	}
	r.Scopes = scopes
	earliest, e := parseEarliest(t.Earliest)
	if e != nil {
		return Record{}, errSafe
	}
	if t.ID != "" {
		sub, e := c.verify(ctx, d, t.ID, r.ClientID, nonce, r.Subject)
		if e != nil {
			return Record{}, errSafe
		}
		r.Subject = sub
		r.IDToken = t.ID
	}
	r.AccessToken = t.Access
	if t.Refresh != "" {
		r.RefreshToken = t.Refresh
	}
	r.ExpiresAt = time.Now().Add(time.Duration(seconds) * time.Second)
	r.EarliestRefreshAt = earliest
	r.Generation++
	return r, nil
}

type callbackResult struct{ code, client string }
type browserHandler struct {
	host, state, authorize string
	expectedClient         string
	mu                     sync.Mutex
	used                   bool
	result                 chan callbackResult
}

var rootTemplate = template.Must(template.New("root").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>SIWC proof</title></head><body><a href="{{.}}">Continue with ChatGPT</a></body></html>`))

func (h *browserHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	reject := func() { w.WriteHeader(http.StatusBadRequest); _, _ = io.WriteString(w, "Request rejected.") }
	if r.Method != "GET" || r.Host != h.host || r.URL.RawPath != "" {
		reject()
		return
	}
	if r.URL.Path == "/" {
		if r.URL.RawQuery != "" {
			reject()
			return
		}
		_ = rootTemplate.Execute(w, h.authorize)
		return
	}
	if r.URL.Path != "/auth/callback" || len(r.URL.RawQuery) > 16384 {
		reject()
		return
	}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		reject()
		return
	}
	for _, v := range q {
		if len(v) != 1 {
			reject()
			return
		}
	}
	if !secureEqual(q.Get("state"), h.state) {
		reject()
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.used {
		reject()
		return
	}
	h.used = true
	code, client := q.Get("code"), q.Get("client_id")
	if client == "" {
		client = h.expectedClient
	}
	if q.Get("error") != "" || code == "" || client == "" || client == dynamicClient {
		h.result <- callbackResult{}
		reject()
		return
	}
	h.result <- callbackResult{code, client}
	_, _ = io.WriteString(w, "Callback received. You may close this window.")
}

// Login opens a loopback browser landing page. It never launches a browser or prints URLs.
// Callers must obtain explicit live-operation approval before calling this function.
func Login(ctx context.Context, port int) error {
	if port != 1455 {
		return errSafe
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	s, e := openStore()
	if e != nil {
		return errSafe
	}
	defer s.close()
	r, e := s.load()
	if e != nil {
		return errSafe
	}
	if r.HostID == "" {
		r.HostID, e = hostID()
		if e != nil || s.save(r) != nil {
			return errSafe
		}
	}
	ln, e := net.Listen("tcp4", "127.0.0.1:1455")
	if e != nil {
		return errSafe
	}
	defer ln.Close()
	c := newClient()
	d, e := c.discover(ctx)
	if e != nil {
		return errSafe
	}
	state, e := randomString()
	if e != nil {
		return errSafe
	}
	nonce, e := randomString()
	if e != nil {
		return errSafe
	}
	verifier, e := randomString()
	if e != nil {
		return errSafe
	}
	challenge := sha256.Sum256([]byte(verifier))
	redirect := "http://127.0.0.1:1455/auth/callback"
	client := r.ClientID
	if client == "" {
		client = dynamicClient
	}
	q := url.Values{"client_id": {client}, "agent_name_hint": {"atuin-ai"}, "ext_agent_host_id": {r.HostID}, "response_type": {"code"}, "redirect_uri": {redirect}, "scope": {"openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"}, "resource": {resource}, "state": {state}, "nonce": {nonce}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	if r.ClientID != "" {
		q.Del("agent_name_hint")
		// The unauthenticated loopback landing page must never expose a
		// retained ID token, even as an authorization hint. Use account selection.
	}
	h := &browserHandler{host: "127.0.0.1:1455", state: state, expectedClient: r.ClientID, authorize: d.Authorization + "?" + q.Encode(), result: make(chan callbackResult, 1)}
	server := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16384, ErrorLog: log.New(io.Discard, "", 0)}
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ln) }()
	var cb callbackResult
	select {
	case cb = <-h.result:
	case <-ctx.Done():
		return errSafe
	case <-done:
		return errSafe
	}
	if cb.code == "" || (r.ClientID != "" && r.ClientID != cb.client) {
		return errSafe
	}
	r.ClientID = cb.client
	// Persist the independently issued registration even if the token request fails.
	if s.save(r) != nil {
		return errSafe
	}
	r, e = c.exchange(ctx, d, url.Values{"grant_type": {"authorization_code"}, "client_id": {r.ClientID}, "code": {cb.code}, "redirect_uri": {redirect}, "code_verifier": {verifier}, "resource": {resource}}, r, nonce, true)
	if e != nil {
		return errSafe
	}
	return s.save(r)
}

// Refresh rotates tokens atomically under the exclusive store lock.
func Refresh(ctx context.Context) error {
	s, e := openStore()
	if e != nil {
		return errSafe
	}
	defer s.close()
	r, e := s.load()
	if e != nil || r.RefreshToken == "" || r.ClientID == "" || r.ClientID == dynamicClient || time.Now().Before(r.EarliestRefreshAt) || time.Until(r.ExpiresAt) > 5*time.Minute {
		return errSafe
	}
	c := newClient()
	d, e := c.discover(ctx)
	if e != nil {
		return errSafe
	}
	r, e = c.exchange(ctx, d, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {r.RefreshToken}, "client_id": {r.ClientID}, "resource": {resource}}, r, "", false)
	if e != nil {
		return errSafe
	}
	return s.save(r)
}

// Logout revokes the refresh token before removing local tokens, retaining registration.
func Logout(ctx context.Context) error {
	s, e := openStore()
	if e != nil {
		return errSafe
	}
	defer s.close()
	r, e := s.load()
	if e != nil {
		return errSafe
	}
	if r.RefreshToken != "" {
		c := newClient()
		d, e := c.discover(ctx)
		if e != nil {
			return errSafe
		}
		if c.request(ctx, "POST", d.Revoke, url.Values{"token": {r.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {r.ClientID}}, nil) != nil {
			return errSafe
		}
	}
	return s.save(Record{HostID: r.HostID, ClientID: r.ClientID, Subject: r.Subject, Generation: r.Generation + 1})
}
