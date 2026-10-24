// Package lfs keeps Git LFS objects in the PC's local Space. git-lfs runs
// modlock as its transfer agent, and the agent reaches the daemon, which
// serves one UnixFS object that every repository on the PC shares: LFS names
// each object by its content's hash, so repositories that hold the same file
// store it once.
package lfs

import (
	"context"
	"io"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/paralin/modlock/space"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	git_lfs "github.com/s4wave/spacewave/core/git/lfs"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

// ObjectKey is the Space's UnixFS object that holds the LFS objects.
const ObjectKey = "git-lfs"

// Server serves the Space's LFS object to agents once the daemon has opened
// the Space. Calls that arrive earlier wait for it.
type Server struct {
	// opened closes once root or err is set.
	opened chan struct{}
	// root serves the object's root directory.
	root srpc.Invoker
	// err is why the object could not be opened.
	err error
}

// NewServer returns a Server waiting for its Space.
func NewServer() *Server {
	return &Server{opened: make(chan struct{})}
}

// Register serves the object on mux.
func (s *Server) Register(mux srpc.Mux) error {
	return resource_server.NewResourceServer(s).Register(mux)
}

// Serve opens the LFS object in sp and serves it until ctx ends. A failure
// to open it fails each agent with its cause.
func (s *Server) Serve(ctx context.Context, le *logrus.Entry, sp *space.Space) error {
	// Open the object, or report why agents cannot use it.
	fs, err := sp.OpenFS(ctx, ObjectKey)
	if err != nil {
		s.Fail(err)
		return err
	}
	defer fs.Release()

	// Serve its root until the daemon stops.
	s.root = resource_unixfs.NewFSHandleObjectResource(le, fs.Handle, nil, fs.World, fs.Key, fs.Type, nil).GetMux()
	close(s.opened)
	<-ctx.Done()
	return nil
}

// Fail fails each agent with err, for a daemon that cannot open its Space.
// Call it at most once, and not with Serve.
func (s *Server) Fail(err error) {
	s.err = err
	close(s.opened)
}

// InvokeMethod serves a call on the object's root once it is open.
func (s *Server) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	select {
	case <-s.opened:
	case <-strm.Context().Done():
		return false, strm.Context().Err()
	}
	if s.err != nil {
		return false, s.err
	}
	return s.root.InvokeMethod(serviceID, methodID, strm)
}

// RunAgent serves git-lfs's transfer protocol on r and w, storing objects
// through the daemon that client reaches. Downloads land in tmpDir, which
// must be on the same filesystem as the repository's LFS store. A daemon that
// cannot serve the object answers git-lfs with the cause.
func RunAgent(ctx context.Context, client srpc.Client, tmpDir string, r io.Reader, w io.Writer) error {
	// Reach the object's root through the daemon's resource service.
	res, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(client))
	if err != nil {
		return Refuse(r, w, err)
	}
	defer res.Release()
	ref := res.AccessRootResource()
	defer ref.Release()
	rootClient, err := ref.GetClient()
	if err != nil {
		return Refuse(r, w, err)
	}

	// Serve git-lfs over it.
	store := git_lfs.NewUnixFSStore(s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient), res)
	return git_lfs.NewAgent(store, tmpDir).Run(ctx, r, w)
}

// Refuse answers git-lfs's first request with cause and returns cause, so
// git-lfs shows why the agent cannot run.
func Refuse(r io.Reader, w io.Writer, cause error) error {
	_ = git_lfs.Refuse(r, w, cause) // cause is the one to report
	return cause
}
