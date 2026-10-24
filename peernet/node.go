// Package peernet runs a modlock peer: a PC's key on Spacewave's peer network,
// which links directly to other PCs over WebRTC. A signaling service,
// hyperline.gg by default, introduces two peers so they can open the link; no
// traffic passes through it. It admits a peer while that peer holds a socket
// signed by its key and carrying the creator's sign-in.
package peernet

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// SigningEnv separates the signatures a peer sends hyperline.gg from requests
// to any other service its key could sign for.
const SigningEnv = "hyperline"

// Node is a running peer: the bus that dials and serves peer streams under
// the node's peer ID.
type Node struct {
	// transport owns the node's links, signaling and child bus.
	transport *transport.SessionTransport
}

// GetBus returns the bus that dials and serves peer streams.
func (n *Node) GetBus() bus.Bus {
	return n.transport.GetChildBus()
}

// GetPeerId returns the node's peer ID, derived from its key.
func (n *Node) GetPeerId() peer.ID {
	return n.transport.GetPeerID()
}

// Run starts a peer for key that signals through the service at origin and
// runs serve with it until serve returns or the peer stops.
// A peer the service does not admit yet retries signaling without delaying
// serve; links that need no signaling keep working meanwhile.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	key crypto.PrivKey,
	origin string,
	serve func(ctx context.Context, node *Node) error,
) error {
	// Build the transport; its Execute owns the child bus and every link.
	t, err := transport.NewSessionTransport(le, nil, key, origin, SigningEnv)
	if err != nil {
		return errors.Wrap(err, "start peer")
	}

	// Run the transport, and stop serve when it exits.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	exited := make(chan error, 1)
	go func() {
		exited <- t.Execute(ctx)
		cancel()
	}()

	// Serve once the node is ready, then stop the transport. Its failure
	// explains the cancellation serve saw, so it takes precedence.
	err = t.AwaitReady(ctx)
	if err == nil {
		err = serve(ctx, &Node{transport: t})
	}
	cancel()
	if failed := <-exited; failed != nil && !errors.Is(failed, context.Canceled) {
		return failed
	}
	return err
}
