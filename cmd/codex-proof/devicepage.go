package main

import (
	"context"
	"errors"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Jghh42/atuin-ai/internal/codexproof"
)

// This optional page is merely private code delivery for the test operator.
// OAuth never redirects here. Production device login needs no callback tunnel.
type devicePage struct {
	mu     sync.Mutex
	prompt *codexproof.DevicePrompt
	now    func() time.Time
}

var deviceTemplate = template.Must(template.New("device").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>atuin-ai Codex device login</title></head><body><h1>Codex device login test</h1><p>Open <a href="{{.VerificationURI}}" target="_blank" rel="noopener noreferrer">OpenAI's Codex activation page</a> in a new tab, then enter this code:</p><pre>{{.UserCode}}</pre><p>Choose the intended account/workspace. Never share this code or include it in screenshots or logs. No OAuth callback or credential import is used.</p></body></html>`))

func (p *devicePage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Host != "127.0.0.1:1455" || r.Method != "GET" || r.URL.Path != "/" || r.URL.RawPath != "" || r.URL.RawQuery != "" {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Request rejected.")
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prompt == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, "Preparing the device login. Reload shortly.")
		return
	}
	if !p.now().Before(p.prompt.ExpiresAt) {
		w.WriteHeader(http.StatusGone)
		io.WriteString(w, "Device login has expired.")
		return
	}
	_ = deviceTemplate.Execute(w, p.prompt)
}

func startDevicePage(ctx context.Context) (func(codexproof.DevicePrompt) error, func(), error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:1455")
	if err != nil {
		return nil, nil, errors.New("cannot open device display")
	}
	page := &devicePage{now: time.Now}
	server := &http.Server{Handler: page, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10, ErrorLog: log.New(io.Discard, "", 0)}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	go func() {
		select {
		case <-ctx.Done():
			_ = server.Close()
		case <-done:
		}
	}()
	display := func(p codexproof.DevicePrompt) error {
		if p.VerificationURI != "https://auth.openai.com/codex/device" || p.UserCode == "" {
			return errors.New("invalid device prompt")
		}
		page.mu.Lock()
		defer page.mu.Unlock()
		page.prompt = &p
		return nil
	}
	stop := func() { _ = server.Close(); <-done; page.mu.Lock(); page.prompt = nil; page.mu.Unlock() }
	return display, stop, nil
}
