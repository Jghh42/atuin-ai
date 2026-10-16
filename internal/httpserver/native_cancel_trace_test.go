package httpserver

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// All observations belong to one request, including on failure/cleanup. Never
// log headers or payloads. The listener observes real TCP, not simulated errors.
type nativeCancelCycle struct {
	start            time.Time
	release          chan struct{}
	started          chan struct{}
	upstreamCanceled chan time.Time
	upstreamDone     chan struct{}
	firstText        chan time.Time
	firstTCPWrite    chan time.Time
	timedOut         chan nativeWriteTimeout
	downstreamDone   chan struct{}
	mu               sync.Mutex
	events           []string
}

func newNativeCancelCycle() *nativeCancelCycle {
	return &nativeCancelCycle{
		start: time.Now(), release: make(chan struct{}), started: make(chan struct{}),
		upstreamCanceled: make(chan time.Time, 1), upstreamDone: make(chan struct{}),
		firstText: make(chan time.Time, 1), firstTCPWrite: make(chan time.Time, 1),
		timedOut: make(chan nativeWriteTimeout, 1), downstreamDone: make(chan struct{}),
	}
}

func (c *nativeCancelCycle) add(format string, args ...any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, fmt.Sprintf("%s %s", time.Since(c.start), fmt.Sprintf(format, args...)))
}

func (c *nativeCancelCycle) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.events, "\n")
}

type nativeTraceWriter struct {
	http.ResponseWriter
	cycle *nativeCancelCycle
}

func (w nativeTraceWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w nativeTraceWriter) Write(p []byte) (int, error) {
	if strings.HasPrefix(string(p[:min(len(p), 12)]), "event: text") {
		select {
		case w.cycle.firstText <- time.Now():
		default:
		}
		w.cycle.add("downstream text Write start bytes=%d", len(p))
	}
	n, err := w.ResponseWriter.Write(p)
	if err != nil {
		w.cycle.add("downstream frame Write error: %v", err)
	}
	return n, err
}
func (w nativeTraceWriter) FlushError() error {
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if err != nil {
		w.cycle.add("downstream Flush error: %v", err)
	}
	return err
}

// A client receive buffer alone does NOT bound how many frames the server can
// enqueue. Bound the accepted socket's send buffer before net/http uses it.
// Use this on both local TCP legs; observe only the downstream leg.
type nativeCancelListener struct {
	net.Listener
	current *atomic.Pointer[nativeCancelCycle]
}

func (l nativeCancelListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tcp := conn.(*net.TCPConn)
	if err := tcp.SetWriteBuffer(1024); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if l.current == nil {
		return conn, nil
	}
	return &nativeCancelConn{Conn: conn, cycle: l.current.Load()}, nil
}

type nativeWriteTimeout struct {
	start, end, deadline time.Time
	written, requested   int
}

type nativeCancelConn struct {
	net.Conn
	cycle    *nativeCancelCycle
	mu       sync.Mutex
	deadline time.Time
}

func (c *nativeCancelConn) Write(p []byte) (int, error) {
	start := time.Now()
	c.mu.Lock()
	deadline := c.deadline
	c.mu.Unlock()
	if c.cycle != nil && len(p) >= 4096 {
		select {
		case c.cycle.firstTCPWrite <- start:
		default:
		}
	}
	n, err := c.Conn.Write(p)
	end := time.Now()
	c.cycle.add("TCP Write bytes=%d/%d elapsed=%s err=%v", n, len(p), end.Sub(start), err)
	if ne, ok := err.(net.Error); ok && ne.Timeout() && c.cycle != nil {
		select {
		case c.cycle.timedOut <- nativeWriteTimeout{start, end, deadline, n, len(p)}:
		default:
		}
	}
	return n, err
}
func (c *nativeCancelConn) SetWriteDeadline(d time.Time) error {
	c.mu.Lock()
	c.deadline = d
	c.mu.Unlock()
	return c.Conn.SetWriteDeadline(d)
}
