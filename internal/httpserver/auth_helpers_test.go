package httpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"time"
)

// testToken is a synthetic, validly formatted local client token.
const testToken = "aia1_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// newRequest is httptest.NewRequest as the Atuin CLI sends it: loopback Host
// and the client bearer token. Unauthenticated cases build requests directly.
func newRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Host = "127.0.0.1:17391"
	r.Header.Set("Authorization", "Bearer "+testToken)
	return r
}

// authorizedPost is http.Post with the client bearer token.
func authorizedPost(url, contentType string, body io.Reader) (*http.Response, error) {
	r, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Authorization", "Bearer "+testToken)
	return (&http.Client{Timeout: 30 * time.Second}).Do(r)
}
