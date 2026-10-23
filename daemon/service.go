package daemon

import (
	"context"
	"os"
	"time"
)

// service serves the Daemon service for one running daemon.
type service struct {
	// version is the release the daemon belongs to.
	version string
	// started is when the daemon started.
	started time.Time
	// counted counts the connected clients.
	counted *listener
	// stop ends the daemon.
	stop context.CancelFunc
}

// GetStatus returns the daemon's process, release and clients.
func (s *service) GetStatus(context.Context, *GetStatusRequest) (*GetStatusResponse, error) {
	clients, _ := s.counted.open()
	return &GetStatusResponse{
		Pid:           uint32(os.Getpid()),
		Version:       s.version,
		StartedUnixMs: s.started.UnixMilli(),
		Clients:       uint32(clients),
	}, nil
}

// Stop ends the daemon. Its reply can be lost as the socket closes, so a
// client waits for the daemon's lock instead.
func (s *service) Stop(context.Context, *StopRequest) (*StopResponse, error) {
	s.stop()
	return &StopResponse{}, nil
}

// _ is a type assertion
var _ SRPCDaemonServer = (*service)(nil)
