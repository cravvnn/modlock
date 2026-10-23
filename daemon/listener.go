package daemon

import (
	"net"

	"github.com/aperturerobotics/util/broadcast"
)

// listener counts the connections it accepted that are still open, so the
// daemon can tell when no client has used it for a while.
type listener struct {
	net.Listener

	// bcast guards conns and wakes waiters when it changes.
	bcast broadcast.Broadcast
	// conns holds the open connections.
	conns map[*conn]struct{}
}

// newListener wraps lis to count its open connections.
func newListener(lis net.Listener) *listener {
	return &listener{Listener: lis, conns: make(map[*conn]struct{})}
}

// Accept waits for the next connection and counts it until it closes.
func (l *listener) Accept() (net.Conn, error) {
	// Accept from the socket.
	nc, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// Count the connection until it closes.
	c := &conn{Conn: nc, l: l}
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.conns[c] = struct{}{}
		broadcast()
	})
	return c, nil
}

// open returns how many connections are open and a channel closed at the next
// change.
func (l *listener) open() (int, <-chan struct{}) {
	var n int
	var wait <-chan struct{}
	l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		n, wait = len(l.conns), getWaitCh()
	})
	return n, wait
}

// _ is a type assertion
var _ net.Listener = (*listener)(nil)
