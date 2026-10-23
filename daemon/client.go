package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// Client is one modlock process's connection to its PC's daemon.
type Client struct {
	// conn is the socket connection.
	conn net.Conn
	// client opens RPC streams over the connection.
	client srpc.Client
	// status is the daemon's status when the client connected.
	status *GetStatusResponse
}

// Connect connects to the daemon for dir that belongs to release version. It
// starts one with start when none runs, and replaces one of another release.
// start must return once the new daemon listens, as Start does.
func Connect(ctx context.Context, dir, version string, start func(context.Context) error) (*Client, error) {
	// Hold the start lock, so no daemon starts or stops meanwhile.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	lock, err := lockFileContext(ctx, filepath.Join(dir, startName))
	if err != nil {
		return nil, err
	}
	defer lock.Close()

	// Use the running daemon when it belongs to this release.
	c, err := dial(ctx, dir)
	if err == nil && c.status.GetVersion() == version {
		return c, nil
	}

	// Stop a daemon of another release and wait for it to exit.
	if err == nil {
		if err := c.stop(ctx, dir); err != nil {
			return nil, err
		}
	}

	// Start a daemon and connect to it.
	if err := start(ctx); err != nil {
		return nil, errors.Wrap(err, "start the modlock daemon")
	}
	return dial(ctx, dir)
}

// Stop stops the daemon for dir, if one runs, and waits for it to exit. It
// reports whether one was running.
func Stop(ctx context.Context, dir string) (bool, error) {
	// Hold the start lock, so no daemon starts meanwhile.
	lock, err := lockFileContext(ctx, filepath.Join(dir, startName))
	if err != nil {
		return false, err
	}
	defer lock.Close()

	// Stop the daemon that answers.
	c, err := dial(ctx, dir)
	if err != nil {
		return false, nil
	}
	return true, c.stop(ctx, dir)
}

// stop asks the daemon to exit, disconnects, and waits for the daemon to free
// dir's lock.
func (c *Client) stop(ctx context.Context, dir string) error {
	// Ask the daemon to exit; its reply can be lost as it closes.
	_, _ = NewSRPCDaemonClient(c.client).Stop(ctx, &StopRequest{})
	_ = c.Close()

	// Wait for the daemon to free the directory.
	held, err := lockFileContext(ctx, filepath.Join(dir, lockName))
	if err != nil {
		return err
	}
	return held.Close()
}

// dial connects to the daemon listening for dir and reads its status, which
// also shows the daemon accepted the connection.
func dial(ctx context.Context, dir string) (*Client, error) {
	// Connect to the socket.
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, SocketName))
	if err != nil {
		return nil, err
	}

	// Open RPCs over the connection and read the daemon's status.
	mc, err := srpc.NewMuxedConn(conn, true, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	c := &Client{conn: conn, client: srpc.NewClientWithMuxedConn(mc)}
	c.status, err = NewSRPCDaemonClient(c.client).GetStatus(ctx, &GetStatusRequest{})
	if err != nil {
		_ = conn.Close()
		return nil, errors.Wrap(err, "read the modlock daemon's status")
	}
	return c, nil
}

// RPC returns the client that calls the daemon's services.
func (c *Client) RPC() srpc.Client {
	return c.client
}

// Status returns the daemon's status when the client connected.
func (c *Client) Status() *GetStatusResponse {
	return c.status
}

// Close disconnects from the daemon.
func (c *Client) Close() error {
	return c.conn.Close()
}
