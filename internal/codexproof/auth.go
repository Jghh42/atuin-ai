// Source-informed Go adaptation of Pi's Codex device-flow protocol,
// @earendil-works/pi-ai 1.1.0 (MIT, Copyright (c) 2025 Mario Zechner).
// Modified for this isolated bounded probe, private display and locked storage.
// See LICENSES/Pi-MIT.txt and docs/provenance/pi-codex.md.
package codexproof

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	clientID          = "app_EMoamEEZ73f0CkXaXp7hrann"
	authBase          = "https://auth.openai.com"
	deviceRedirectURI = authBase + "/deviceauth/callback"
	verificationURI   = authBase + "/codex/device"
	maxBody           = 1 << 20
)

// DevicePrompt contains a short-lived login secret. Display only to the user;
// never log, persist, or include it in errors.
type DevicePrompt struct {
	UserCode        string
	VerificationURI string
	ExpiresAt       time.Time
}

func (DevicePrompt) String() string   { return "[private Codex device prompt]" }
func (DevicePrompt) GoString() string { return "[private Codex device prompt]" }

type authClient struct {
	http  *http.Client
	base  string
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

func newClient() *authClient {
	return &authClient{
		http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 32768, DisableKeepAlives: true}},
		base: authBase, now: time.Now, sleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return errSafe
			case <-t.C:
				return nil
			}
		},
	}
}
func readLimited(r io.Reader) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, maxBody+1))
	if e != nil || len(b) > maxBody {
		return nil, errSafe
	}
	return b, nil
}
func (c *authClient) post(ctx context.Context, path, contentType, body string) (int, []byte, error) {
	req, e := http.NewRequestWithContext(ctx, "POST", c.base+path, strings.NewReader(body))
	if e != nil {
		return 0, nil, errSafe
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	resp, e := c.http.Do(req)
	if e != nil {
		return 0, nil, errSafe
	}
	defer resp.Body.Close()
	b, e := readLimited(resp.Body)
	if e != nil {
		return 0, nil, errSafe
	}
	return resp.StatusCode, b, nil
}
func goodString(s string) bool { return strings.TrimSpace(s) != "" && len(s) <= 65536 }
func parseInterval(raw json.RawMessage) (time.Duration, error) {
	s := string(raw)
	if len(raw) > 0 && raw[0] == '"' {
		if json.Unmarshal(raw, &s) != nil {
			return 0, errSafe
		}
		s = strings.TrimSpace(s)
	}
	n, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 900 {
		return 0, errSafe
	}
	d := time.Duration(n * float64(time.Second))
	if d < time.Second {
		d = time.Second
	}
	return d, nil
}
func errorCode(b []byte) string {
	var v struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(v.Error, &s) == nil {
		return s
	}
	var o struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(v.Error, &o)
	return o.Code
}

// Login performs only the fixed Codex device flow. It refuses to replace any
// renewable session (including expired access tokens); explicitly Logout first.
func Login(ctx context.Context, display func(DevicePrompt) error) error {
	return login(ctx, display, newClient())
}
func login(ctx context.Context, display func(DevicePrompt) error, c *authClient) error {
	if ctx.Err() != nil || display == nil {
		return errSafe
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	s, e := openStore()
	if e != nil {
		return errSafe
	}
	defer s.close()
	old, e := s.load()
	if e != nil || old.AccessToken != "" || old.RefreshToken != "" {
		return errSafe
	}
	status, b, e := c.post(ctx, "/api/accounts/deviceauth/usercode", "application/json", `{"client_id":"`+clientID+`"}`)
	if e != nil || status != 200 {
		return errSafe
	}
	var device struct {
		ID       string          `json:"device_auth_id"`
		Code     string          `json:"user_code"`
		Interval json.RawMessage `json:"interval"`
	}
	if json.Unmarshal(b, &device) != nil || !goodString(device.ID) || !goodString(device.Code) {
		return errSafe
	}
	interval, e := parseInterval(device.Interval)
	if e != nil {
		return errSafe
	}
	deadline := c.now().Add(15 * time.Minute)
	if display(DevicePrompt{UserCode: device.Code, VerificationURI: verificationURI, ExpiresAt: deadline}) != nil {
		return errSafe
	}
	body, _ := json.Marshal(map[string]string{"device_auth_id": device.ID, "user_code": device.Code})
	for c.now().Before(deadline) {
		if ctx.Err() != nil {
			return errSafe
		}
		status, b, e = c.post(ctx, "/api/accounts/deviceauth/token", "application/json", string(body))
		if e != nil {
			return errSafe
		}
		if status >= 200 && status < 300 {
			var code struct {
				Code     string `json:"authorization_code"`
				Verifier string `json:"code_verifier"`
			}
			if json.Unmarshal(b, &code) != nil || !goodString(code.Code) || !goodString(code.Verifier) {
				return errSafe
			}
			r, e := c.exchange(ctx, url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {code.Code}, "code_verifier": {code.Verifier}, "redirect_uri": {deviceRedirectURI}}, Record{Generation: old.Generation})
			if e != nil {
				return errSafe
			}
			// Once tokens have been issued, complete persistence even if the
			// caller cancels or the device deadline races the exchange result.
			return s.save(r)
		}
		ec := errorCode(b)
		if ec == "slow_down" {
			interval += 5 * time.Second
		} else if status != 403 && status != 404 && ec != "deviceauth_authorization_pending" {
			return errSafe
		}
		remaining := deadline.Sub(c.now())
		if remaining <= 0 {
			return errSafe
		}
		wait := interval
		if wait > remaining {
			wait = remaining
		}
		if c.sleep(ctx, wait) != nil {
			return errSafe
		}
	}
	return errSafe
}

// accountFromJWT decodes untrusted JWT payload metadata, NOT a signature
// verification. Tokens are accepted only from the fixed HTTPS token endpoint.
func accountFromJWT(token string, now time.Time) (string, time.Time, error) {
	if len(token) > 65536 {
		return "", time.Time{}, errSafe
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return "", time.Time{}, errSafe
	}
	for _, p := range parts {
		if _, e := base64.RawURLEncoding.DecodeString(p); e != nil {
			return "", time.Time{}, errSafe
		}
	}
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var headerObject map[string]json.RawMessage
	if json.Unmarshal(header, &headerObject) != nil || headerObject == nil {
		return "", time.Time{}, errSafe
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return "", time.Time{}, errSafe
	}
	var claims struct {
		Auth struct {
			Account string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
		Exp json.RawMessage `json:"exp"`
	}
	if json.Unmarshal(b, &claims) != nil || !goodString(claims.Auth.Account) {
		return "", time.Time{}, errSafe
	}
	if len(claims.Exp) == 0 {
		return "", time.Time{}, errSafe
	}
	var expiry time.Time
	if len(claims.Exp) > 0 {
		// Reject null, strings, fractional dates, and already-expired access tokens.
		n, e := strconv.ParseInt(string(claims.Exp), 10, 64)
		if e != nil || n <= 0 || n > 253402300799 {
			return "", time.Time{}, errSafe
		}
		expiry = time.Unix(n, 0)
		if !expiry.After(now) {
			return "", time.Time{}, errSafe
		}
	}
	return claims.Auth.Account, expiry, nil
}
func (c *authClient) exchange(ctx context.Context, form url.Values, old Record) (Record, error) {
	status, b, e := c.post(ctx, "/oauth/token", "application/x-www-form-urlencoded", form.Encode())
	if e != nil || status != 200 {
		return Record{}, errSafe
	}
	var token struct {
		Access  string          `json:"access_token"`
		Refresh string          `json:"refresh_token"`
		Expires json.RawMessage `json:"expires_in"`
	}
	if json.Unmarshal(b, &token) != nil || !goodString(token.Access) || !goodString(token.Refresh) {
		return Record{}, errSafe
	}
	seconds, e := strconv.ParseInt(string(token.Expires), 10, 64)
	if e != nil || seconds <= 0 || seconds > 31536000 {
		return Record{}, errSafe
	}
	now := c.now()
	account, jwtExpiry, e := accountFromJWT(token.Access, now)
	if e != nil || (old.AccountID != "" && old.AccountID != account) || (old.RefreshToken != "" && old.RefreshToken == token.Refresh) {
		return Record{}, errSafe
	}
	expiry := now.Add(time.Duration(seconds) * time.Second)
	if !jwtExpiry.IsZero() && jwtExpiry.Before(expiry) {
		expiry = jwtExpiry
	}
	return Record{AccessToken: token.Access, RefreshToken: token.Refresh, AccountID: account, ExpiresAt: expiry, Generation: old.Generation + 1}, nil
}

// Refresh serializes refresh-token rotation and durably saves the new token
// before allowing any other operation to use it. It never retries requests.
func Refresh(ctx context.Context) error { return refresh(ctx, newClient()) }
func refresh(ctx context.Context, c *authClient) error {
	if ctx.Err() != nil {
		return errSafe
	}
	s, e := openStore()
	if e != nil {
		return errSafe
	}
	defer s.close()
	old, e := s.load()
	if e != nil || old.RefreshToken == "" || old.AccountID == "" {
		return errSafe
	}
	r, e := c.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {old.RefreshToken}}, old)
	if e != nil {
		return errSafe
	}
	// Once rotation succeeds, persistence must finish even if cancellation races it.
	return s.save(r)
}
