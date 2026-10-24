package gamelink

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// Dial opens a game stream of protocol to the host peer and keeps its link
// until forwarding stops. ctx owns both the dial and the endpoint's lifetime.
// It returns the endpoint only once the host accepts generation and has
// reserved the guest's socket.
func Dial(ctx context.Context, b bus.Bus, protocol protocol.ID, local, host peer.ID, generation uint64) (*Client, error) {
	// Open an unreliable stream to the host, so a lost game packet never
	// delays the ones after it.
	if generation == 0 {
		return nil, errors.New("the game host generation is required")
	}
	mounted, release, err := link.OpenStreamWithPeerEx(ctx, b, protocol, local, host, 0, stream.OpenOpts{Unreliable: true})
	if err != nil {
		return nil, err
	}
	peerStream, ok := mounted.GetStream().(stream.MessageStream)
	if !ok || mounted.GetPeerID() != host {
		mounted.GetStream().Close()
		release()
		return nil, errors.New("the game stream reached a different peer")
	}

	// Finish the handshake before binding the loopback endpoint.
	if err := requestAdmission(ctx, peerStream.Control(), generation); err != nil {
		peerStream.Close()
		release()
		return nil, err
	}
	client, err := open(ctx, peerStream, release)
	if err != nil {
		release()
		return nil, err
	}
	return client, nil
}
