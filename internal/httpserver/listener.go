package httpserver

import (
	"net"
	"sync"
)

// Bound connection goroutines as well as chat handlers. Overload is closed
// immediately, never queued in the application. The OS TCP backlog remains
// finite and OS-managed. HTTP/1 only: no multiplexed plaintext HTTP/2 support.
const maxConnections = 64

type boundedListener struct {
	net.Listener
	slots chan struct{}
}

func (l *boundedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			return &boundedConn{Conn: conn, release: func() { <-l.slots }}, nil
		default:
			_ = conn.Close()
		}
	}
}

type boundedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *boundedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}
