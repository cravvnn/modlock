package peernet

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Service runs a PC's peer with its admission and relay credentials, and
// serves the PeerNet service for it. Its calls wait for Run to load the key,
// so a daemon can register the service before it holds its state directory.
type Service struct {
	// origin is the signaling service.
	origin string
	// keyPath is the peer's key file.
	keyPath string
	// signIn reads the saved sign-in.
	signIn func() SignIn

	// started closes once Run loaded the key and set local and admission.
	started chan struct{}
	// key is the peer's key, which signs its sockets to the service.
	key crypto.PrivKey
	// local is the peer, derived from its key.
	local peer.ID
	// admission holds the peer's admission to signaling.
	admission *Admission
	// ready closes once node is set.
	ready chan struct{}
	// node is the running peer.
	node *Node
	// hosting keeps the daemon to one hosted session at a time, since two
	// would answer the same game streams.
	hosting atomic.Bool
}

// NewService constructs the peer whose key lives at keyPath, signaling at
// origin.
func NewService(origin, keyPath string, signIn func() SignIn) *Service {
	return &Service{
		origin:  origin,
		keyPath: keyPath,
		signIn:  signIn,
		started: make(chan struct{}),
		ready:   make(chan struct{}),
	}
}

// Register serves the PeerNet service on mux.
func (s *Service) Register(mux srpc.Mux) error {
	return SRPCRegisterPeerNet(mux, s)
}

// Run loads the key, creating it on first use, and runs the peer, its
// admission and its probe echo until ctx ends. Only one process may run a
// service for a key file at a time.
func (s *Service) Run(ctx context.Context, le *logrus.Entry) error {
	// Load the key and publish the peer to the service's calls.
	key, err := LoadKey(s.keyPath)
	if err != nil {
		return err
	}
	s.key = key
	s.local, err = peer.IDFromPrivateKey(key)
	if err != nil {
		return err
	}
	s.admission = NewAdmission(s.origin, key, s.signIn)
	close(s.started)

	// Hold the admission beside the peer.
	go s.admission.Run(ctx)
	return Run(ctx, le, key, s.origin, func(ctx context.Context, node *Node) error {
		// Answer probes, then publish the node to the service's calls.
		release, err := node.GetBus().AddController(ctx, NewEcho(s.local), nil)
		if err != nil {
			return err
		}
		defer release()
		s.node = node
		close(s.ready)
		<-ctx.Done()
		return nil
	})
}

// await waits for ch to close or ctx to end.
func await(ctx context.Context, ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetPeer returns the peer and its admission. A call also asks the admission
// to read the sign-in again, since a client may have just signed in.
func (s *Service) GetPeer(ctx context.Context, _ *GetPeerRequest) (*GetPeerResponse, error) {
	if err := await(ctx, s.started); err != nil {
		return nil, err
	}
	s.admission.Wake()
	state, account, failure := s.admission.Snapshot()
	return &GetPeerResponse{
		PeerId:    s.local.String(),
		Origin:    s.origin,
		Admission: state,
		Account:   account,
		Error:     failure,
	}, nil
}

// Probe measures the link to another peer once the local peer is ready.
func (s *Service) Probe(ctx context.Context, req *ProbeRequest) (*ProbeResponse, error) {
	// Read the remote peer and wait for the local one.
	remote, err := peer.IDB58Decode(req.GetPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "read the peer ID")
	}
	if err := await(ctx, s.ready); err != nil {
		return nil, err
	}
	return Probe(ctx, s.node.GetBus(), s.local, remote)
}

// _ is a type assertion.
var _ SRPCPeerNetServer = (*Service)(nil)
