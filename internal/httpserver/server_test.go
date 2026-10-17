package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func start(t *testing.T, cfg config.Config, chat http.Handler) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	var s *Server
	var err error
	source := catalog.New(credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
		return credentials.Credential{Identity: credentials.Identity{Provider: "codex", Account: "test-account", Epoch: 1}, AccessToken: "test-token"}, nil
	}), cfg.DefaultModel)
	if chat == nil {
		s, err = NewWithCatalog(cfg, nil, source)
	} else {
		s, err = buildServer(cfg, nil, source, chat)
	}
	if err != nil {
		t.Fatal(err)
	}
	host, _, err := net.SplitHostPort(s.http.Addr)
	if err != nil || host != "127.0.0.1" {
		t.Fatalf("unsafe address: %s", s.http.Addr)
	}
	s.http.Addr = net.JoinHostPort(host, "0")
	l, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	if addr.IP.String() != "127.0.0.1" || addr.IP.To4() == nil {
		t.Fatal(addr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, l) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(7 * time.Second):
			t.Error("shutdown timed out")
		}
	})
	return "http://" + l.Addr().String(), cancel, done
}

func request(t *testing.T, method, url string, body io.Reader, status int) []byte {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	// Authentication is intentionally absent, not required or rejected.
	req.Header.Set("Authorization", "Bearer deliberately-ignored")
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != status {
		t.Fatalf("%s: got %d want %d: %s", url, res.StatusCode, status, data)
	}
	if res.Header.Get("Content-Type") != "application/json" {
		t.Fatal(res.Header)
	}
	return data
}

func TestRoutesAndModels(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultModel = "gpt-6.1-sol"
	base, _, _ := start(t, cfg, nil)
	body := request(t, "GET", base+"/api/cli/models", nil, 200)
	var got struct {
		Default string          `json:"default"`
		Models  []catalog.Model `json:"models"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Default != "gpt-6.1-sol" || len(got.Models) != 2 || got.Models[0].Alias != "gpt-6.1-sol" || got.Models[1].Alias != "gpt-6-luna" {
		t.Fatalf("%s", body)
	}
	if bytes.Contains(body, []byte("private-upstream")) {
		t.Fatal("upstream IDs leaked")
	}
	request(t, "POST", base+"/api/cli/chat", strings.NewReader(`{}`), 400)
	request(t, "POST", base+"/api/cli/chat", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`), 503)
	request(t, "GET", base+"/api/cli/chat", nil, 405)
	request(t, "POST", base+"/api/cli/models", nil, 405)
	request(t, "HEAD", base+"/api/cli/models", nil, 405)
	for _, path := range []string{"/", "/health", "/api/cli/usage", "/api/cli/models/", "/api/cli/../cli/models"} {
		request(t, "GET", base+path, nil, 404)
	}
}

func TestDefaultFirstModel(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultModel = ""
	base, _, _ := start(t, cfg, nil)
	data := request(t, "GET", base+"/api/cli/models", nil, 200)
	var got struct {
		Default string `json:"default"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Default != "gpt-6.1-sol" {
		t.Fatalf("%s", data)
	}
}

func TestBodyLimits(t *testing.T) {
	base, _, _ := start(t, config.Default(), nil)
	for _, size := range []int{config.MaxRequestBodyBytes, config.MaxRequestBodyBytes + 1} {
		status := 400
		if size > config.MaxRequestBodyBytes {
			status = 413
		}
		request(t, "POST", base+"/api/cli/chat", strings.NewReader(strings.Repeat("x", size)), status)
		// Unknown Content-Length must obey the same bound.
		request(t, "POST", base+"/api/cli/chat", struct{ io.Reader }{strings.NewReader(strings.Repeat("x", size))}, status)
	}
}

func TestSaturationAndRecovery(t *testing.T) {
	entered := make(chan struct{}, config.MaxConcurrentChats)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	base, _, _ := start(t, config.Default(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		writeError(w, 501, "not_implemented", "placeholder")
	}))
	var wg sync.WaitGroup
	for range config.MaxConcurrentChats {
		wg.Add(1)
		go func() { defer wg.Done(); request(t, "POST", base+"/api/cli/chat", strings.NewReader(`{}`), 501) }()
	}
	for range config.MaxConcurrentChats {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("not admitted")
		}
	}
	request(t, "POST", base+"/api/cli/chat", strings.NewReader(`{}`), 503)
	request(t, "GET", base+"/api/cli/models", nil, 200)
	once.Do(func() { close(release) })
	wg.Wait()
	request(t, "POST", base+"/api/cli/chat", strings.NewReader(`{}`), 501)
}

func TestLoopbackOnlyAndShutdown(t *testing.T) {
	base, cancel, _ := start(t, config.Default(), nil)
	request(t, "GET", base+"/api/cli/models", nil, 200)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(base, "http://"))
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil || ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		conn, err := net.DialTimeout("tcp4", net.JoinHostPort(ip.String(), port), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			t.Fatalf("reachable via non-loopback %s", ip)
		}
	}
	cancel()
	// Poll the actual socket, not asynchronous worker status.
	deadline := time.Now().Add(time.Second)
	for {
		conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", port), 50*time.Millisecond)
		if err != nil {
			break
		}
		conn.Close()
		if time.Now().After(deadline) {
			t.Fatal("listener still open after cancellation")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRejectNonLoopbackListener(t *testing.T) {
	s, err := New(config.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Serve(context.Background(), l); err == nil {
		t.Fatal("accepted wildcard listener")
	}
}

func TestGracefulShutdownDrainsActiveRequest(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	base, cancel, _ := start(t, config.Default(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		if r.Context().Err() != nil {
			t.Error("active request cancelled before drain")
		}
		writeError(w, 501, "not_implemented", "placeholder")
	}))
	finished := make(chan struct{})
	go func() { defer close(finished); request(t, "POST", base+"/api/cli/chat", strings.NewReader(`{}`), 501) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	cancel()
	waitForListenerClose(t, strings.TrimPrefix(base, "http://"))
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("handler did not drain")
	}
}

func waitForListenerClose(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		c, err := net.DialTimeout("tcp4", address, 50*time.Millisecond)
		if err != nil {
			return
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatal("listener did not close")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestForcedShutdownCancelsActiveRequest(t *testing.T) {
	entered, cancelled := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	s, err := newServer(config.Default(), nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(cancelled)
		<-release
	}))
	if err != nil {
		t.Fatal(err)
	}
	s.http.Addr = "127.0.0.1:0"
	l, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, l) }()
	clientDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 7 * time.Second}
		res, err := client.Post("http://"+l.Addr().String()+"/api/cli/chat", "application/json", strings.NewReader(`{}`))
		if res != nil {
			res.Body.Close()
		}
		clientDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	started := time.Now()
	cancel()
	select {
	case <-cancelled:
	case <-time.After(6 * time.Second):
		t.Fatal("handler context not cancelled")
	}
	if time.Since(started) < 4*time.Second {
		t.Fatal("did not allow graceful drain")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return")
	}
	select {
	case err := <-clientDone:
		if err == nil {
			t.Fatal("active connection not forcibly closed")
		}
	case <-time.After(time.Second):
		t.Fatal("client still connected")
	}
}
