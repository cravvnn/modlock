package peernet

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// ProbeProtocolID identifies probe streams between modlock peers. The stream
// is an unreliable message stream, like a game's, and the answering peer
// returns each probe packet to its sender.
const ProbeProtocolID = protocol.ID("modlock/probe/1")

// probeMagic opens each probe packet, so the echo answers nothing else.
var probeMagic = []byte("mlprobe1")

// probePackets is how many packets one probe sends.
const probePackets = 5

// probeWait bounds the wait for one packet's echo.
const probeWait = 2 * time.Second

// echoLimit bounds how long one probe stream is answered, so a stream that is
// never closed stops costing the answering PC its upload.
const echoLimit = 15 * time.Second

// Probe opens a probe stream from local to remote on b and returns the
// shortest round trip of its packets and whether the link is relayed.
func Probe(ctx context.Context, b bus.Bus, local, remote peer.ID) (*ProbeResponse, error) {
	// Open the stream, which links the two peers when they are not linked.
	mounted, release, err := link.OpenStreamWithPeerEx(ctx, b, ProbeProtocolID, local, remote, 0, stream.OpenOpts{Unreliable: true})
	if err != nil {
		return nil, err
	}
	defer release()
	messages, ok := mounted.GetStream().(stream.MessageStream)
	if !ok {
		mounted.GetStream().Close()
		return nil, errors.New("probe stream carries no messages")
	}
	defer messages.Close()
	relayed := link.GetPath(mounted.GetLink()) == link.PathRelay

	// Send each packet and wait for its echo, ignoring stale ones.
	var best time.Duration
	buf := make([]byte, 64)
	for i := range uint32(probePackets) {
		packet := binary.BigEndian.AppendUint32(bytes.Clone(probeMagic), i)
		sent := time.Now()
		if _, err := messages.Write(packet); err != nil {
			return nil, err
		}
		rtt, err := awaitEcho(ctx, messages, packet, buf, sent)
		if err != nil {
			return nil, err
		}
		if best == 0 || rtt < best {
			best = rtt
		}
	}
	return &ProbeResponse{RttUs: uint32(best.Microseconds()), Relayed: relayed}, nil
}

// awaitEcho reads messages until packet comes back and returns its round trip
// since sent.
func awaitEcho(ctx context.Context, messages stream.MessageStream, packet, buf []byte, sent time.Time) (time.Duration, error) {
	// Close the stream if the echo does not arrive in time.
	ctx, cancel := context.WithTimeout(ctx, probeWait)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = messages.Close() })
	defer stop()

	// Read until this packet's echo.
	for {
		n, err := messages.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return 0, errors.New("the peer did not answer the probe")
			}
			return 0, err
		}
		if bytes.Equal(buf[:n], packet) {
			return time.Since(sent), nil
		}
	}
}

// Echo answers probe streams from any peer to the local peer for as long as
// its controller runs on the bus.
type Echo struct {
	// local is the peer that answers.
	local peer.ID
}

// NewEcho constructs the echo for the local peer.
func NewEcho(local peer.ID) *Echo {
	return &Echo{local: local}
}

// GetControllerInfo identifies the echo.
func (e *Echo) GetControllerInfo() *controller.Info {
	return controller.NewInfo("modlock/peernet/echo", controller.MustParseVersion("0.0.1"), "probe echo")
}

// Execute does nothing; the echo answers streams as they arrive.
func (e *Echo) Execute(context.Context) error { return nil }

// Close does nothing; each stream ends with its own context.
func (e *Echo) Close() error { return nil }

// HandleDirective accepts probe streams to the local peer.
func (e *Echo) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	d, ok := inst.GetDirective().(link.HandleMountedStream)
	if !ok || d.HandleMountedStreamLocalPeerID() != e.local || d.HandleMountedStreamProtocolID() != ProbeProtocolID {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver([]link.MountedStreamHandler{e})), nil
}

// HandleMountedStream returns each probe packet to its sender until the stream
// closes or the limit passes.
func (e *Echo) HandleMountedStream(ctx context.Context, mounted link.MountedStream) error {
	messages, ok := mounted.GetStream().(stream.MessageStream)
	if !ok {
		return errors.New("probe stream carries no messages")
	}
	go func() {
		// Close the stream at the limit.
		ctx, cancel := context.WithTimeout(ctx, echoLimit)
		defer cancel()
		stop := context.AfterFunc(ctx, func() { _ = messages.Close() })
		defer stop()
		defer messages.Close()

		// Answer each probe packet.
		buf := make([]byte, 64)
		for {
			n, err := messages.Read(buf)
			if err != nil {
				return
			}
			if bytes.HasPrefix(buf[:n], probeMagic) {
				_, _ = messages.Write(buf[:n])
			}
		}
	}()
	return nil
}

// _ is a type assertion.
var (
	_ controller.Controller     = (*Echo)(nil)
	_ link.MountedStreamHandler = (*Echo)(nil)
)
