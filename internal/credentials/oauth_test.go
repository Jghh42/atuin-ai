package credentials

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDeviceProtocolPendingSlowdownAndPKCE(t *testing.T) {
	now := testNow
	polls := 0
	waits := []time.Duration{}
	shown := 0
	c := OAuthClient{Now: func() time.Time { return now }, Wait: func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		now = now.Add(d)
		return nil
	}, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "auth.openai.com" || r.Method != "POST" {
			t.Fatal("unexpected destination")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("unbounded HTTP request")
		}
		b, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			if string(b) != `{"client_id":"`+ClientID+`"}` {
				t.Fatal("wrong registration")
			}
			return reply(200, `{"device_auth_id":"device-test","user_code":"ABCD-EFGH","interval":"2"}`), nil
		case "/api/accounts/deviceauth/token":
			var body map[string]string
			_ = json.Unmarshal(b, &body)
			if body["device_auth_id"] != "device-test" || body["user_code"] != "ABCD-EFGH" {
				t.Fatal("bad pending code")
			}
			polls++
			switch polls {
			case 1:
				return reply(403, `{"error":"deviceauth_authorization_pending"}`), nil
			case 2:
				return reply(403, `{"error":{"code":"slow_down"}}`), nil
			default:
				return reply(200, `{"authorization_code":"auth-code-test","code_verifier":"`+strings.Repeat("v", 43)+`"}`), nil
			}
		case "/oauth/token":
			form, _ := url.ParseQuery(string(b))
			if form.Get("grant_type") != "authorization_code" || form.Get("client_id") != ClientID || form.Get("redirect_uri") != deviceRedirect || form.Get("code") != "auth-code-test" || form.Get("code_verifier") != strings.Repeat("v", 43) || len(form) != 5 {
				t.Fatal("wrong PKCE exchange")
			}
			return reply(200, tokenJSON("account-test", "refresh-new", now.Add(time.Hour))), nil
		}
		t.Fatal("unexpected endpoint")
		return nil, nil
	})}
	r, e := c.login(context.Background(), func(p DevicePrompt) error {
		shown++
		if p.UserCode != "ABCD-EFGH" {
			t.Fatal("wrong prompt")
		}
		return nil
	})
	if e != nil || r.Account != "account-test" || shown != 1 || polls != 3 || len(waits) != 2 || waits[0] != 2*time.Second || waits[1] != 7*time.Second {
		t.Fatal("device flow mismatch", e)
	}
}
func TestDeviceErrorsAndPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"denied403", 403, `{"error":"access_denied"}`, ErrDenied}, {"denied200", 200, `{"error":{"code":"access_denied"}}`, ErrDenied}, {"expired404", 404, `{"error":"expired_token"}`, ErrExpired}, {"unknown404", 404, `{"error":"bad_request"}`, ErrOAuth}, {"malformed403", 403, `not json`, ErrOAuth}, {"missingcode", 200, `{}`, ErrOAuth}, {"badpkce", 200, `{"authorization_code":"test","code_verifier":"short"}`, ErrOAuth}, {"duplicate", 200, `{"authorization_code":"test","authorization_code":"other","code_verifier":"test"}`, ErrOAuth}, {"redirect", 302, `{}`, ErrOAuth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := OAuthClient{Now: func() time.Time { return testNow }, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return reply(200, `{"device_auth_id":"device-test","user_code":"TEST-CODE","interval":1}`), nil
				}
				return reply(tc.status, tc.body), nil
			})}
			_, e := c.login(context.Background(), func(DevicePrompt) error { return nil })
			if !errors.Is(e, tc.want) || calls != 2 {
				t.Fatal("wrong safe failure", e, calls)
			}
		})
	}
}
func TestDeviceExpiryCancellationAndAcquireBounds(t *testing.T) {
	t.Run("expiry", func(t *testing.T) {
		now := testNow
		polls := 0
		c := OAuthClient{Now: func() time.Time { return now }, Wait: func(ctx context.Context, d time.Duration) error { now = now.Add(d); return nil }, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "usercode") {
				return reply(200, `{"device_auth_id":"test","user_code":"TEST","interval":900}`), nil
			}
			polls++
			return reply(404, `{}`), nil
		})}
		_, e := c.login(context.Background(), func(DevicePrompt) error { return nil })
		if e != ErrExpired || polls != 1 {
			t.Fatal("expiry not bounded", e, polls)
		}
	})
	t.Run("cancel-acquire", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c := OAuthClient{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			cancel()
			<-r.Context().Done()
			return nil, r.Context().Err()
		})}
		_, e := c.login(ctx, func(DevicePrompt) error { t.Fatal("display after failed acquire"); return nil })
		if e == nil {
			t.Fatal("accepted cancellation")
		}
	})
	t.Run("cancel-wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c := OAuthClient{Now: func() time.Time { return testNow }, Wait: func(ctx context.Context, d time.Duration) error { cancel(); return ctx.Err() }, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "usercode") {
				return reply(200, `{"device_auth_id":"test","user_code":"TEST","interval":1}`), nil
			}
			return reply(403, `{}`), nil
		})}
		_, e := c.login(ctx, func(DevicePrompt) error { return nil })
		if e == nil {
			t.Fatal("accepted cancelled poll")
		}
	})
	for _, body := range []string{`{"device_auth_id":"test","user_code":"bad\ncode","interval":1}`, `{"device_auth_id":"test","user_code":"TEST","interval":null}`, `{"device_auth_id":"test","user_code":"TEST","interval":-1}`, `{"device_auth_id":"test","user_code":"TEST","interval":901}`, strings.Repeat("x", maxRecordBytes+1), `{"device_auth_id":"test","user_code":"TEST","interval":1,"error":"denied"}`} {
		c := OAuthClient{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return reply(200, body), nil })}
		if _, e := c.login(context.Background(), func(DevicePrompt) error { t.Fatal("invalid prompt displayed"); return nil }); e == nil {
			t.Fatal("invalid acquire accepted")
		}
	}
}
func TestTokenValidationAndAccountBinding(t *testing.T) {
	old := activeRecord(testNow.Add(time.Hour))
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"valid", tokenJSON("account-test", "refresh-new", testNow.Add(time.Hour)), 200, true},
		{"changedaccount", tokenJSON("other-account", "refresh-new", testNow.Add(time.Hour)), 200, false},
		{"unchangedrefresh", tokenJSON("account-test", old.Refresh, testNow.Add(time.Hour)), 200, false},
		{"missingrefresh", `{"access_token":"test","expires_in":3600}`, 200, false},
		{"invalidgrant", `{"error":"invalid_grant","access_token":"ignored"}`, 400, false},
		{"erroreven200", `{"error":"invalid_grant"}`, 200, false},
		{"expired", tokenJSON("account-test", "refresh-new", testNow), 200, false},
		{"badauthheader", tokenJSON("bad\r\naccount", "refresh-new", testNow.Add(time.Hour)), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := OAuthClient{Now: func() time.Time { return testNow }, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(r.Body)
				v, _ := url.ParseQuery(string(b))
				if v.Get("client_id") != ClientID || v.Get("grant_type") != "refresh_token" || v.Get("refresh_token") != old.Refresh || len(v) != 3 {
					t.Fatal("bad refresh form")
				}
				return reply(tc.status, tc.body), nil
			})}
			r, e := c.refresh(context.Background(), old)
			if (e == nil) != tc.ok {
				t.Fatal("token validation mismatch", e)
			}
			if tc.ok && r.ExpiresAt != testNow.Add(time.Hour) {
				t.Fatal("wrong expiry")
			}
		})
	}
	for _, exp := range []string{`"1792155600"`, `null`, `1792155600.5`, `1e10`, `-1`} {
		payload := `{"https://api.openai.com/auth":{"chatgpt_account_id":"account-test"},"exp":` + exp + `}`
		token := "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".c2ln"
		if _, _, e := accessClaims(token); e == nil {
			t.Fatal("invalid claim date accepted")
		}
	}
}
func TestTokenExpiryIsEarlierBoundAndStrictNumeric(t *testing.T) {
	for _, duration := range []time.Duration{20 * time.Minute, 2 * time.Hour} {
		c := OAuthClient{Now: func() time.Time { return testNow }, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return reply(200, tokenJSON("account-test", "replacement", testNow.Add(duration))), nil
		})}
		r, e := c.exchange(context.Background(), nil, Record{})
		want := duration
		if want > time.Hour {
			want = time.Hour
		}
		if e != nil || !r.ExpiresAt.Equal(testNow.Add(want)) {
			t.Fatal("expiry bound mismatch", e)
		}
	}
	for _, value := range []string{`"3600"`, `null`, `3600.5`, `1e4`, `0`, `31536001`} {
		body := `{"access_token":"` + jwt("account-test", testNow.Add(time.Hour)) + `","refresh_token":"replacement","expires_in":` + value + `}`
		c := OAuthClient{Now: func() time.Time { return testNow }, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return reply(200, body), nil })}
		if _, e := c.exchange(context.Background(), nil, Record{}); e == nil {
			t.Fatal("invalid response expiry accepted")
		}
	}
}
func TestNativeTransportIsBoundedWithoutRawIdleDiagnostics(t *testing.T) {
	transport := NewTransport()
	defer transport.CloseIdleConnections()
	if !transport.DisableKeepAlives || transport.Protocols == nil || !transport.Protocols.HTTP1() || transport.Protocols.HTTP2() || transport.Protocols.UnencryptedHTTP2() || transport.MaxResponseHeaderBytes != 32<<10 || transport.MaxConnsPerHost != 6 || transport.TLSHandshakeTimeout != 10*time.Second || transport.ResponseHeaderTimeout != 10*time.Second {
		t.Fatal("unsafe native transport policy")
	}
}
func TestOAuthNoRedirectAndFiniteDeadline(t *testing.T) {
	calls := 0
	c := OAuthClient{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 21*time.Second {
			t.Fatal("HTTP bound missing")
		}
		res := reply(302, `{}`)
		res.Header.Set("Location", "https://unreviewed.invalid/")
		return res, nil
	})}
	if _, e := c.login(context.Background(), func(DevicePrompt) error { return nil }); e == nil || calls != 1 {
		t.Fatal("redirect followed or accepted")
	}
}
