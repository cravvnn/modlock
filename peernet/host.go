package peernet

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"net"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/paralin/modlock/gamelink"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// GameProtocolID identifies game streams between modlock peers, which
// gamelink forwards. The handshake's generation is the hosted session, so
// only a guest holding the join code reaches the server.
const GameProtocolID = protocol.ID("modlock/game/1")

// JoinCode names a hosted session: the host's peer and the session's secret.
type JoinCode struct {
	// Host is the hosting PC's peer.
	Host peer.ID
	// Session is the session's nonzero secret.
	Session uint64
}

// String formats the code as the peer, a dash and the session in hex. Peer
// IDs are base58, which has no dash.
func (c JoinCode) String() string {
	return c.Host.String() + "-" + strconv.FormatUint(c.Session, 16)
}

// ParseJoinCode reads a code that JoinCode.String formatted.
func ParseJoinCode(code string) (JoinCode, error) {
	// Split the peer from the session.
	host, session, ok := strings.Cut(strings.TrimSpace(code), "-")
	if !ok {
		return JoinCode{}, errors.New("a join code is the host's peer, a dash and the session")
	}
	id, err := peer.IDB58Decode(host)
	if err != nil {
		return JoinCode{}, errors.Wrap(err, "read the join code's peer")
	}

	// Read the session, which is never zero.
	secret, err := strconv.ParseUint(session, 16, 64)
	if err != nil || secret == 0 {
		return JoinCode{}, errors.New("the join code's session is not valid")
	}
	return JoinCode{Host: id, Session: secret}, nil
}

// hosting answers game streams to the local peer that name its session,
// forwarding each guest to the game server, while its controller runs.
type hosting struct {
	// ctx ends the hosted session and every guest's forwarding.
	ctx context.Context
	// local is the hosting peer.
	local peer.ID
	// session is the join code's secret.
	session uint64
	// target is the game server.
	target *net.UDPAddr
	// events receives each guest that joins or leaves.
	events func(*HostEvent)
}

// GetControllerInfo identifies the hosted session.
func (h *hosting) GetControllerInfo() *controller.Info {
	return controller.NewInfo("modlock/peernet/hosting", controller.MustParseVersion("0.0.1"), "hosted game session")
}

// Execute does nothing; guests arrive as streams.
func (h *hosting) Execute(context.Context) error { return nil }

// Close does nothing; forwarding ends with ctx.
func (h *hosting) Close() error { return nil }

// HandleDirective accepts game streams to the local peer.
func (h *hosting) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	d, ok := inst.GetDirective().(link.HandleMountedStream)
	if !ok || d.HandleMountedStreamLocalPeerID() != h.local || d.HandleMountedStreamProtocolID() != GameProtocolID {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver([]link.MountedStreamHandler{h})), nil
}

// HandleMountedStream forwards a guest that names the session to the server
// until the guest or the session ends, and closes any other stream.
func (h *hosting) HandleMountedStream(_ context.Context, mounted link.MountedStream) error {
	// Take the guest's game stream.
	messages, ok := mounted.GetStream().(stream.MessageStream)
	if !ok {
		mounted.GetStream().Close()
		return errors.New("the game stream carries no messages")
	}
	guest := mounted.GetPeerID().String()
	go func() {
		// Refuse a guest without the join code.
		control := messages.Control()
		generation, err := gamelink.ReadGeneration(h.ctx, control)
		if err != nil || generation != h.session {
			messages.Close()
			return
		}

		// Forward the guest, reporting when it joins and leaves.
		h.events(&HostEvent{Body: &HostEvent_Joined{Joined: guest}})
		_ = gamelink.Forward(h.ctx, h.target, messages, func() error { return gamelink.AcceptGeneration(h.ctx, control) })
		h.events(&HostEvent{Body: &HostEvent_Left{Left: guest}})
	}()
	return nil
}

// Host serves the local game server to guests holding the join code it sends
// first, until the call ends.
func (s *Service) Host(req *HostRequest, strm SRPCPeerNet_HostStream) error {
	// Hold the daemon's one session.
	ctx := strm.Context()
	if req.GetPort() == 0 || req.GetPort() > 65535 {
		return errors.New("name the game server's port")
	}
	if err := await(ctx, s.ready); err != nil {
		return err
	}
	if !s.hosting.CompareAndSwap(false, true) {
		return errors.New("this PC already hosts a session")
	}
	defer s.hosting.Store(false)

	// Pick the session's secret.
	var secret [8]byte
	for binary.BigEndian.Uint64(secret[:]) == 0 {
		_, _ = rand.Read(secret[:])
	}
	code := JoinCode{Host: s.local, Session: binary.BigEndian.Uint64(secret[:])}

	// Answer guests, sending each change in order.
	events := make(chan *HostEvent, 16)
	events <- &HostEvent{Body: &HostEvent_JoinCode{JoinCode: code.String()}}
	target := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(req.GetPort())}
	h := &hosting{ctx: ctx, local: s.local, session: code.Session, target: target, events: func(event *HostEvent) {
		select {
		case events <- event:
		case <-ctx.Done():
		}
	}}
	release, err := s.node.GetBus().AddController(ctx, h, nil)
	if err != nil {
		return err
	}
	defer release()

	// Send events until the call ends.
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-events:
			if err := strm.Send(event); err != nil {
				return err
			}
		}
	}
}

// Join links to the session the code names and serves it to the local game
// on a loopback port until the game leaves or the call ends.
func (s *Service) Join(req *JoinRequest, strm SRPCPeerNet_JoinStream) error {
	// Read the code and wait for the local peer.
	ctx := strm.Context()
	code, err := ParseJoinCode(req.GetCode())
	if err != nil {
		return err
	}
	if err := await(ctx, s.ready); err != nil {
		return err
	}

	// Link to the host and wait for it to accept the session.
	client, err := gamelink.Dial(ctx, s.node.GetBus(), GameProtocolID, s.local, code.Host, code.Session)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := strm.Send(&JoinEvent{Body: &JoinEvent_Address{Address: client.Address()}}); err != nil {
		return err
	}

	// Report the game's arrival, then hold until it leaves.
	select {
	case <-client.Arrived():
		if err := strm.Send(&JoinEvent{Body: &JoinEvent_Arrived{Arrived: true}}); err != nil {
			return err
		}
	case <-client.Done():
	}
	if err := client.Wait(); err != nil && !errors.Is(err, gamelink.ErrGameLeft) && ctx.Err() == nil {
		return err
	}
	return nil
}
