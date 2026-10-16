package httpserver

import (
	"net"
	"testing"
	"time"
)

func TestConnectionAdmissionAndRecovery(t *testing.T) {
	raw, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	l := &boundedListener{Listener: raw, slots: make(chan struct{}, maxConnections)}
	dial := func() net.Conn {
		t.Helper()
		c, err := net.DialTimeout("tcp4", raw.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	accepted := make([]net.Conn, 0, maxConnections)
	for range maxConnections {
		dial()
		c, err := l.Accept()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		accepted = append(accepted, c)
	}
	overloaded := dial()
	result := make(chan net.Conn, 1)
	go func() { c, _ := l.Accept(); result <- c }()
	_ = overloaded.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	if _, err := overloaded.Read(b[:]); err == nil {
		t.Fatal("overloaded connection not closed")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("overloaded connection queued")
	}
	accepted[0].Close()
	accepted[0].Close() // release must be idempotent
	dial()
	select {
	case c := <-result:
		if c == nil {
			t.Fatal("replacement not admitted")
		}
		c.Close()
	case <-time.After(time.Second):
		t.Fatal("slot not recovered")
	}
}
