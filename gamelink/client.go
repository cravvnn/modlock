// Package gamelink carries a game's UDP traffic between two PCs over a peer
// stream. The guest's game connects to a loopback endpoint; the host forwards
// each admitted guest to its game server with a socket of its own, so the
// server sees every guest as a separate client. Each stream opens with a
// handshake that names the server process the guest expects, and game packets
// flow only once the host accepts it.
package gamelink

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/net/stream"
	datagram "github.com/s4wave/spacewave/net/stream/datagram"
)

// quietLimit is how long an arrived game may stop sending before the endpoint
// treats it as gone. A connected Deadlock client sends every tick; the limit
// leaves room for a slow map load, which can pause it.
const quietLimit = 2 * time.Minute

// ErrGameLeft reports that the game stopped sending after it arrived: the
// player left the server or quit Deadlock.
var ErrGameLeft = errors.New("Left the game")

// Client owns one loopback endpoint until the peer disconnects, the game
// leaves, or Close returns. It serves one server process and is not reused.
type Client struct {
	// address is the loopback endpoint the game connects to.
	address string
	// arrived closes on the game's first packet.
	arrived chan struct{}
	// cancel stops forwarding with its cause.
	cancel context.CancelCauseFunc
	// done closes once forwarding has stopped.
	done chan struct{}
	// err is the forwarding result, set before done closes.
	err error
}

// Open takes ownership of an admitted game stream and binds the loopback
// endpoint for it. The caller then points the game at Address; no process is
// launched.
func Open(ctx context.Context, peer stream.MessageStream) (*Client, error) {
	return open(ctx, peer, nil)
}

// open binds the loopback endpoint and forwards it to peer until ctx ends or
// the game leaves, then calls release.
func open(ctx context.Context, peer stream.MessageStream, release func()) (*Client, error) {
	// Bind the endpoint.
	socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		peer.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancelCause(ctx)
	client := &Client{address: socket.LocalAddr().String(), arrived: make(chan struct{}), cancel: cancel, done: make(chan struct{})}
	local := &arrivalStream{MessageStream: peer, arrived: client.arrived}

	// Forward until either side stops or the game goes quiet.
	go func() {
		// Release the link and report the result once forwarding stops.
		defer close(client.done)
		if release != nil {
			defer release()
		}
		defer cancel(nil)

		// Forward, ending with ErrGameLeft when the game goes quiet.
		go local.watchQuiet(ctx, quietLimit, func() { cancel(ErrGameLeft) })
		client.err = datagram.ForwardLocal(ctx, socket, local)
		if cause := context.Cause(ctx); errors.Is(cause, ErrGameLeft) {
			client.err = cause
		}
	}()
	return client, nil
}

// Address is the game's connect endpoint, valid until Done closes.
func (c *Client) Address() string { return c.address }

// Arrived closes when the game sends its first packet to Address.
func (c *Client) Arrived() <-chan struct{} { return c.arrived }

// Done closes after forwarding has stopped.
func (c *Client) Done() <-chan struct{} { return c.done }

// Wait returns the forwarding result after both directions stop.
// ErrGameLeft means the game went quiet after it arrived.
func (c *Client) Wait() error { <-c.done; return c.err }

// Close cancels forwarding and waits for sockets and pumps to close.
func (c *Client) Close() { c.cancel(nil); <-c.done }

// arrivalStream closes arrived on the first local packet forwarded to the host
// and records when the game last sent one. Only the forwarder's single socket
// pump calls Write.
type arrivalStream struct {
	stream.MessageStream
	// arrived closes on the first packet.
	arrived chan struct{}
	// last is the UnixNano time of the latest local packet.
	last atomic.Int64
}

// Write marks arrival and activity, then forwards the packet.
func (s *arrivalStream) Write(packet []byte) (int, error) {
	s.last.Store(time.Now().UnixNano())
	select {
	case <-s.arrived:
	default:
		close(s.arrived)
	}
	return s.MessageStream.Write(packet)
}

// watchQuiet calls left once the arrived game has sent nothing for limit. The
// timer runs to each packet's known expiry instead of sampling.
func (s *arrivalStream) watchQuiet(ctx context.Context, limit time.Duration, left func()) {
	// Wait for the game to arrive.
	select {
	case <-ctx.Done():
		return
	case <-s.arrived:
	}

	// Leave once the game has been quiet for limit.
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		quiet := time.Since(time.Unix(0, s.last.Load()))
		if quiet >= limit {
			left()
			return
		}
		timer.Reset(limit - quiet)
	}
}
