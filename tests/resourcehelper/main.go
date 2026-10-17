// resourcehelper is a test-only process. Never shipped or linked into the server.
// setup seeds a fresh synthetic store through Manager.Login; serve composes the
// real application and offers numeric post-GC diagnostics over inherited pipes.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"runtime/metrics"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Jghh42/atuin-ai/internal/application"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/httpserver"
)

type synthetic struct {
	Token   string `json:"token"`
	Account string `json:"account"`
}
type setupTransport struct {
	fake synthetic
	step int
}

func (t *setupTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Deny default: no Dial, proxy, fallback, or real transport exists here.
	if r.Method != "POST" || r.URL.Scheme != "https" || r.URL.Host != "auth.openai.com" || r.URL.RawQuery != "" {
		return nil, errors.New("setup contract")
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 4097))
	if e != nil || len(b) > 4096 {
		return nil, errors.New("setup body")
	}
	var body string
	switch t.step {
	case 0:
		if r.URL.Path != "/api/accounts/deviceauth/usercode" || string(b) != `{"client_id":"`+credentials.ClientID+`"}` {
			return nil, errors.New("setup registration")
		}
		body = `{"device_auth_id":"resource-device","user_code":"RESOURCE-FAKE","interval":1}`
	case 1:
		var v map[string]string
		if r.URL.Path != "/api/accounts/deviceauth/token" || json.Unmarshal(b, &v) != nil || len(v) != 2 || v["device_auth_id"] != "resource-device" || v["user_code"] != "RESOURCE-FAKE" {
			return nil, errors.New("setup poll")
		}
		body = `{"authorization_code":"resource-code","code_verifier":"` + strings.Repeat("v", 43) + `"}`
	case 2:
		v, e := url.ParseQuery(string(b))
		if e != nil || r.URL.Path != "/oauth/token" || len(v) != 5 || v.Get("grant_type") != "authorization_code" || v.Get("client_id") != credentials.ClientID || v.Get("code") != "resource-code" || v.Get("code_verifier") != strings.Repeat("v", 43) || v.Get("redirect_uri") != "https://auth.openai.com/deviceauth/callback" {
			return nil, errors.New("setup exchange")
		}
		data, _ := json.Marshal(map[string]any{"access_token": t.fake.Token, "refresh_token": "resource-fake-refresh", "expires_in": 7200})
		body = string(data)
	default:
		return nil, errors.New("setup unexpected call")
	}
	t.step++
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

// Companion-only TCP instrumentation observes the ACTUAL default five-second
// partial write timeout. Small socket buffers make backpressure finite; shipping
// measurements never use this listener or any timeout/runtime injection.
type writeWitness struct {
	StartNS    int64 `json:"start_ns"`
	EndNS      int64 `json:"end_ns"`
	DeadlineNS int64 `json:"deadline_ns"`
	Requested  int   `json:"requested"`
	Written    int   `json:"written"`
}
type observedListener struct {
	net.Listener
	mu      sync.Mutex
	active  int
	timeout *writeWitness
}
type observedConn struct {
	net.Conn
	owner    *observedListener
	once     sync.Once
	deadline time.Time
}

func (l *observedListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e != nil {
		return nil, e
	}
	tcp, ok := c.(*net.TCPConn)
	if !ok {
		c.Close()
		return nil, errors.New("companion TCP required")
	}
	if e = tcp.SetWriteBuffer(4096); e != nil {
		c.Close()
		return nil, e
	}
	l.mu.Lock()
	l.active++
	l.mu.Unlock()
	return &observedConn{Conn: c, owner: l}, nil
}
func (c *observedConn) SetWriteDeadline(t time.Time) error {
	c.owner.mu.Lock()
	c.deadline = t
	c.owner.mu.Unlock()
	return c.Conn.SetWriteDeadline(t)
}
func (c *observedConn) Write(p []byte) (int, error) {
	c.owner.mu.Lock()
	deadline := c.deadline
	c.owner.mu.Unlock()
	start := time.Now()
	n, e := c.Conn.Write(p)
	end := time.Now()
	if failure, ok := e.(net.Error); ok && failure.Timeout() {
		c.owner.mu.Lock()
		c.owner.timeout = &writeWitness{StartNS: start.UnixNano(), EndNS: end.UnixNano(), DeadlineNS: deadline.UnixNano(), Requested: len(p), Written: n}
		c.owner.mu.Unlock()
	}
	return n, e
}
func (c *observedConn) Close() error {
	e := c.Conn.Close()
	c.once.Do(func() { c.owner.mu.Lock(); c.owner.active--; c.owner.mu.Unlock() })
	return e
}
func (l *observedListener) stats() map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	return map[string]any{"active_tcp": l.active, "write_timeout": l.timeout}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("mode required")
	}
	path, e := application.StateDirectory()
	if e != nil {
		return e
	}
	cfg := config.Default()
	if os.Args[1] == "setup" {
		if len(os.Args) != 3 {
			return errors.New("setup arguments")
		}
		b, e := os.ReadFile(os.Args[2])
		if e != nil {
			return e
		}
		var fake synthetic
		if json.Unmarshal(b, &fake) != nil || fake.Token == "" || fake.Account == "" {
			return errors.New("synthetic input")
		}
		t := &setupTransport{fake: fake}
		app, e := application.NewWithOptions(path, cfg, application.Options{Transport: t})
		if e != nil {
			return e
		}
		e = app.Manager.Login(context.Background(), func(credentials.DevicePrompt) error { return nil })
		closeErr := app.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if t.step != 3 {
			return errors.New("setup count")
		}
		return nil
	}
	if os.Args[1] != "serve" || len(os.Args) != 2 {
		return errors.New("unknown mode")
	}
	// All default production transport, manager, catalog, native parser/runner and
	// server composition. No shortened timeouts or injected provider.
	app, e := application.NewWithOptions(path, cfg, application.Options{})
	if e != nil {
		return e
	}
	defer app.Close()
	server, e := httpserver.NewWithRuntime(cfg, nil, app.HTTP)
	if e != nil {
		return e
	}
	listener, e := server.Listen()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	observed := &observedListener{Listener: listener}
	go func() { done <- server.Serve(ctx, observed) }()
	encoder := json.NewEncoder(os.Stdout)
	if e := encoder.Encode(map[string]bool{"companion_ready": true}); e != nil {
		return e
	}
	commands := make(chan string)
	go func() {
		defer close(commands)
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			select {
			case commands <- s.Text():
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case e := <-done:
			return e
		case command, ok := <-commands:
			if !ok {
				stop()
				return <-done
			}
			if command == "witness" {
				if e := encoder.Encode(observed.stats()); e != nil {
					return e
				}
				continue
			}
			if command != "snapshot" {
				return errors.New("unknown pipe command")
			}
			// ONLY companion retention diagnostics, never production RSS evidence.
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/gc/heap/objects:objects"}, {Name: "/sched/goroutines:goroutines"}}
			metrics.Read(samples)
			if e := encoder.Encode(map[string]any{"goroutines": runtime.NumGoroutine(), "heap_alloc": m.HeapAlloc, "heap_objects": m.HeapObjects, "stack_inuse": m.StackInuse, "heap_inuse": m.HeapInuse, "metric_heap_bytes": samples[0].Value.Uint64(), "metric_heap_objects": samples[1].Value.Uint64(), "metric_goroutines": samples[2].Value.Uint64()}); e != nil {
				return e
			}
		}
	}
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "resource helper failed")
		os.Exit(1)
	}
}
