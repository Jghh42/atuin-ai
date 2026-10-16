// Source-informed Go adaptation of Pi AI 1.1.0's device OAuth protocol.
// MIT, Copyright (c) 2025 Mario Zechner. See LICENSES/Pi-MIT.txt and
// docs/provenance/pi-codex.md. No browser/SIWC flow or runtime is imported.
package credentials

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
	authBase       = "https://auth.openai.com"
	ActivationURL  = authBase + "/codex/device"
	deviceRedirect = authBase + "/deviceauth/callback"
)

// DevicePrompt is a pending secret. Display its fields only on the interactive
// login user's terminal; never persist it or send it to a logger.
type DevicePrompt struct{ UserCode string }

func (DevicePrompt) String() string   { return "[private device prompt]" }
func (DevicePrompt) GoString() string { return "[private device prompt]" }

type deviceAuthorization struct {
	ID       string          `json:"device_auth_id"`
	Code     string          `json:"user_code"`
	Interval json.RawMessage `json:"interval"`
}

func (deviceAuthorization) String() string   { return "[private device authorization]" }
func (deviceAuthorization) GoString() string { return "[private device authorization]" }

type authorizationCode struct {
	Code     string `json:"authorization_code"`
	Verifier string `json:"code_verifier"`
}

func (authorizationCode) String() string   { return "[private authorization code]" }
func (authorizationCode) GoString() string { return "[private authorization code]" }

type tokenResponse struct {
	Access  string          `json:"access_token"`
	Refresh string          `json:"refresh_token"`
	Expires json.RawMessage `json:"expires_in"`
}

func (tokenResponse) String() string   { return "[private token response]" }
func (tokenResponse) GoString() string { return "[private token response]" }

// OAuthClient has no configurable endpoints. Transport/clock/wait are test seams.
type OAuthClient struct {
	Transport http.RoundTripper
	Now       func() time.Time
	Wait      func(context.Context, time.Duration) error
}

func (c OAuthClient) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
func (c OAuthClient) wait(ctx context.Context, d time.Duration) error {
	if c.Wait != nil {
		return c.Wait(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// NewTransport supplies bounded native HTTPS transport for auth, catalog and
// inference. Callers retain streaming timeouts; redirects are refused by clients.
func NewTransport() *http.Transport {
	// net/http's idle HTTP/1 connection diagnostics can log raw unsolicited
	// bytes. Do not pool secret-bearing upstream connections or enable HTTP/2's
	// separate diagnostics. This also bounds transport goroutine lifetimes.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 32 << 10, MaxConnsPerHost: 6,
		DisableKeepAlives: true, Protocols: protocols,
	}
}
func (c OAuthClient) post(ctx context.Context, path, contentType, body string) (int, []byte, error) {
	ctx, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, authBase+path, strings.NewReader(body))
	if e != nil {
		return 0, nil, ErrOAuth
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "atuin-ai/native-codex")
	transport := c.Transport
	if transport == nil {
		return 0, nil, ErrOAuth
	} // production composition must explicitly supply it
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return 0, nil, ErrOAuth
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, maxRecordBytes+1))
	if e != nil || len(b) > maxRecordBytes {
		return 0, nil, ErrOAuth
	}
	return res.StatusCode, b, nil
}
func oauthError(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	var obj map[string]json.RawMessage
	if decodeJSON(b, &obj, false) != nil || obj == nil {
		return "", ErrOAuth
	}
	raw, ok := obj["error"]
	if !ok {
		return "", nil
	}
	var code string
	if json.Unmarshal(raw, &code) == nil && code != "" {
		return code, nil
	}
	var nested struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(raw, &nested) != nil || nested.Code == "" {
		return "", ErrOAuth
	}
	return nested.Code, nil
}
func interval(raw json.RawMessage) (time.Duration, error) {
	if len(raw) == 0 {
		return 5 * time.Second, nil
	}
	s := string(raw)
	if raw[0] == '"' {
		if json.Unmarshal(raw, &s) != nil {
			return 0, ErrOAuth
		}
		s = strings.TrimSpace(s)
	}
	n, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 900 {
		return 0, ErrOAuth
	}
	d := time.Duration(n * float64(time.Second))
	if d < time.Second {
		d = time.Second
	}
	return d, nil
}
func validUserCode(s string) bool {
	if len(s) < 1 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
func validVerifier(s string) bool {
	if len(s) < 43 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~') {
			return false
		}
	}
	return true
}
func (c OAuthClient) login(ctx context.Context, display func(DevicePrompt) error) (Record, error) {
	if ctx.Err() != nil || display == nil {
		return Record{}, ErrOAuth
	}
	ctx, stop := context.WithTimeout(ctx, 15*time.Minute)
	defer stop()
	deadline := c.now().Add(15 * time.Minute)
	status, b, e := c.post(ctx, "/api/accounts/deviceauth/usercode", "application/json", `{"client_id":"`+ClientID+`"}`)
	ec, parseErr := oauthError(b)
	if e != nil || status != 200 || parseErr != nil || ec != "" {
		return Record{}, ErrOAuth
	}
	var device deviceAuthorization
	if decodeJSON(b, &device, false) != nil || !validSecret(device.ID) || !validUserCode(device.Code) {
		return Record{}, ErrOAuth
	}
	delay, e := interval(device.Interval)
	if e != nil || !c.now().Before(deadline) {
		return Record{}, ErrExpired
	}
	if display(DevicePrompt{device.Code}) != nil {
		return Record{}, ErrOAuth
	}
	body, _ := json.Marshal(map[string]string{"device_auth_id": device.ID, "user_code": device.Code})
	for c.now().Before(deadline) {
		if ctx.Err() != nil {
			return Record{}, ErrOAuth
		}
		status, b, e = c.post(ctx, "/api/accounts/deviceauth/token", "application/json", string(body))
		if e != nil {
			return Record{}, ErrOAuth
		}
		if !c.now().Before(deadline) {
			return Record{}, ErrExpired
		}
		ec, e = oauthError(b)
		if e != nil {
			return Record{}, ErrOAuth
		}
		// Explicit errors take precedence over HTTP pending statuses AND success.
		switch ec {
		case "access_denied", "authorization_declined":
			return Record{}, ErrDenied
		case "expired_token", "device_code_expired":
			return Record{}, ErrExpired
		case "slow_down":
			delay += 5 * time.Second
		case "authorization_pending", "deviceauth_authorization_pending":
		case "":
			if status >= 200 && status < 300 {
				var code authorizationCode
				if decodeJSON(b, &code, false) != nil || !validSecret(code.Code) || !validVerifier(code.Verifier) {
					return Record{}, ErrOAuth
				}
				return c.exchange(ctx, url.Values{"grant_type": {"authorization_code"}, "client_id": {ClientID}, "code": {code.Code}, "code_verifier": {code.Verifier}, "redirect_uri": {deviceRedirect}}, Record{})
			}
			if status != 403 && status != 404 {
				return Record{}, ErrOAuth
			}
		default:
			return Record{}, ErrOAuth
		}
		remaining := deadline.Sub(c.now())
		if remaining <= 0 {
			return Record{}, ErrExpired
		}
		wait := delay
		if wait > remaining {
			wait = remaining
		}
		if c.wait(ctx, wait) != nil {
			return Record{}, ErrOAuth
		}
	}
	return Record{}, ErrExpired
}

type accessMetadata struct {
	Auth struct {
		Account string `json:"chatgpt_account_id"`
	} `json:"https://api.openai.com/auth"`
	Exp json.RawMessage `json:"exp"`
}

func (accessMetadata) String() string   { return "[private access metadata]" }
func (accessMetadata) GoString() string { return "[private access metadata]" }

// accessClaims DECODEs untrusted access-token metadata; it is NOT OIDC signature
// verification. Tokens originate only from the fixed HTTPS token endpoint.
func accessClaims(token string) (string, time.Time, error) {
	if !validSecret(token) {
		return "", time.Time{}, ErrOAuth
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", time.Time{}, ErrOAuth
	}
	for _, p := range parts {
		if p == "" {
			return "", time.Time{}, ErrOAuth
		}
		if _, e := base64.RawURLEncoding.DecodeString(p); e != nil {
			return "", time.Time{}, ErrOAuth
		}
	}
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var h map[string]json.RawMessage
	if decodeJSON(header, &h, false) != nil || h == nil {
		return "", time.Time{}, ErrOAuth
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims accessMetadata
	if decodeJSON(payload, &claims, false) != nil || !validAtom(claims.Auth.Account, 256) {
		return "", time.Time{}, ErrOAuth
	}
	exp, e := strconv.ParseInt(string(claims.Exp), 10, 64)
	if e != nil || exp < 1577836800 || exp > 253402300799 {
		return "", time.Time{}, ErrOAuth
	}
	return claims.Auth.Account, time.Unix(exp, 0).UTC(), nil
}
func (c OAuthClient) exchange(ctx context.Context, form url.Values, old Record) (Record, error) {
	status, b, e := c.post(ctx, "/oauth/token", "application/x-www-form-urlencoded", form.Encode())
	if e != nil {
		return Record{}, ErrOAuth
	}
	ec, e := oauthError(b)
	if e != nil {
		return Record{}, ErrOAuth
	}
	if ec == "invalid_grant" {
		return Record{}, ErrReauth
	}
	if status != 200 || ec != "" {
		return Record{}, ErrOAuth
	}
	var token tokenResponse
	if decodeJSON(b, &token, false) != nil || !validSecret(token.Access) || !validSecret(token.Refresh) {
		return Record{}, ErrOAuth
	}
	seconds, e := strconv.ParseInt(string(token.Expires), 10, 64)
	if e != nil || seconds <= 0 || seconds > 31536000 {
		return Record{}, ErrOAuth
	}
	now := c.now()
	account, expiry, e := accessClaims(token.Access)
	if e != nil || !expiry.After(now) || old.Account != "" && account != old.Account || old.Refresh != "" && old.Refresh == token.Refresh {
		return Record{}, ErrReauth
	}
	if limit := now.Add(time.Duration(seconds) * time.Second); limit.Before(expiry) {
		expiry = limit
	}
	r := emptyRecord()
	r.State = "active"
	r.Account = account
	r.Access = token.Access
	r.Refresh = token.Refresh
	r.IssuedAt = now
	r.SavedAt = now
	r.ExpiresAt = expiry
	return r, nil
}
func (c OAuthClient) refresh(ctx context.Context, old Record) (Record, error) {
	return c.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "client_id": {ClientID}, "refresh_token": {old.Refresh}}, old)
}
