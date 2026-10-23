package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
)

// Serve runs the daemon of release version for dir. It takes the directory's
// lock, listens on its socket, calls ready, and serves the Daemon service and
// the rest of mux until ctx ends, a client stops it, or no client has stayed
// connected for idle. It returns ErrRunning when another daemon holds dir.
func Serve(ctx context.Context, dir, version string, idle time.Duration, mux srpc.Mux, ready func()) error {
	// Take the directory and bind its socket.
	lis, release, err := bindStarting(ctx, dir)
	if err != nil {
		return err
	}

	// Serve until stopped, then close the socket before letting clients in.
	stopping, err := serve(ctx, dir, version, idle, lis, mux, ready)
	release()
	if stopping != nil {
		_ = stopping.Close()
	}
	return err
}

// serve accepts clients on lis until ctx ends or the daemon is idle. When it
// stops for idleness it returns the start lock, which the caller frees after
// closing the socket.
func serve(ctx context.Context, dir, version string, idle time.Duration, lis net.Listener, mux srpc.Mux, ready func()) (*os.File, error) {
	// Register the Daemon service beside the caller's.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	counted := newListener(lis)
	svc := &service{version: version, started: time.Now(), counted: counted, stop: cancel}
	if err := SRPCRegisterDaemon(mux, svc); err != nil {
		return nil, err
	}

	// Serve the clients in the background, counting their connections.
	errCh := make(chan error, 1)
	go func() { errCh <- srpc.AcceptMuxedListener(ctx, counted, srpc.NewServer(mux), nil) }()
	ready()

	// Stop once no client has been connected for idle.
	for {
		open, changed := counted.open()
		var idleCh <-chan time.Time
		if open == 0 {
			idleCh = time.After(idle)
		}
		select {
		case <-ctx.Done():
			return nil, nil
		case err := <-errCh:
			return nil, err
		case <-changed:
		case <-idleCh:
			if stopping, err := stopIdle(ctx, dir, counted); stopping != nil || err != nil {
				return stopping, err
			}
		}
	}
}

// bindStarting binds dir as bind does while holding the start lock, unless
// the client that launched this daemon holds it.
func bindStarting(ctx context.Context, dir string) (net.Listener, func(), error) {
	// Create the directory, and bind at once when the starter holds the lock.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	if os.Getenv(startedEnv) != "" {
		_ = os.Unsetenv(startedEnv)
		return bind(dir)
	}

	// Hold the start lock only while binding.
	start, err := lockFileContext(ctx, filepath.Join(dir, startName))
	if err != nil {
		return nil, nil, err
	}
	defer start.Close()
	return bind(dir)
}

// bind takes dir's daemon lock and listens on its socket, removing one a
// stopped daemon left. release closes the socket, removes it and frees the
// lock.
func bind(dir string) (net.Listener, func(), error) {
	// Take the directory, or report the daemon that holds it.
	held, err := lockFile(filepath.Join(dir, lockName), false)
	if err == errLocked {
		return nil, nil, ErrRunning
	}
	if err != nil {
		return nil, nil, err
	}

	// Listen on the socket in place of any one left behind.
	path := filepath.Join(dir, SocketName)
	_ = os.Remove(path)
	lis, err := net.Listen("unix", path)
	if err != nil {
		_ = held.Close()
		return nil, nil, err
	}
	return lis, func() {
		_ = lis.Close()
		_ = os.Remove(path)
		_ = held.Close()
	}, nil
}

// stopIdle takes the start lock so no client is connecting, and returns it
// when the daemon is still idle and should stop.
func stopIdle(ctx context.Context, dir string, counted *listener) (*os.File, error) {
	// Wait out any client that is connecting.
	start, err := lockFileContext(ctx, filepath.Join(dir, startName))
	if err != nil {
		return nil, err
	}

	// Stop only when no client connected meanwhile.
	if open, _ := counted.open(); open != 0 {
		_ = start.Close()
		return nil, nil
	}
	return start, nil
}
