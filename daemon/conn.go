package daemon

import "net"

// conn is one accepted connection that leaves its listener's count when it
// closes.
type conn struct {
	net.Conn

	// l is the listener that counts this connection.
	l *listener
}

// Close closes the connection and stops counting it.
func (c *conn) Close() error {
	c.l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if _, ok := c.l.conns[c]; ok {
			delete(c.l.conns, c)
			broadcast()
		}
	})
	return c.Conn.Close()
}

// _ is a type assertion
var _ net.Conn = (*conn)(nil)
