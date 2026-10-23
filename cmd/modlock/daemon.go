package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/paralin/modlock/daemon"
	"github.com/pkg/errors"
)

// daemonIdle is how long the daemon stays up with no modlock process using it.
const daemonIdle = 10 * time.Minute

// daemonCommand runs, shows and stops the background process that holds this
// PC's mod store and peer.
func daemonCommand(out *printer) *cli.Command {
	return &cli.Command{
		Name:  "daemon",
		Usage: "run the background process that holds this PC's mod store; modlock starts it when needed",
		Flags: []cli.Flag{
			&cli.DurationFlag{Name: "idle", Value: daemonIdle, Usage: "stop after this long with no modlock process using it"},
		},
		Action: func(c *cli.Context) error {
			// Find the state directory.
			dir, err := daemon.Dir()
			if err != nil {
				return err
			}

			// Serve it, unless another daemon already does.
			err = daemon.Serve(c.Context, dir, Version, c.Duration("idle"), srpc.NewMux(), daemon.Ready)
			if errors.Is(err, daemon.ErrRunning) {
				out.note("a modlock daemon already runs; modlock daemon status shows it")
				return nil
			}
			return err
		},
		Subcommands: []*cli.Command{
			{
				Name:  "status",
				Usage: "start the daemon if it is not running and show it",
				Action: func(c *cli.Context) error {
					// Connect, starting the daemon when needed.
					client, err := connectDaemon(c.Context)
					if err != nil {
						return err
					}
					defer client.Close()

					// Show the daemon's process, release and clients.
					status := client.Status()
					out.field("pid", strconv.FormatUint(uint64(status.GetPid()), 10))
					out.field("release", status.GetVersion())
					out.field("since", time.UnixMilli(status.GetStartedUnixMs()).Format(time.DateTime))
					out.field("clients", strconv.FormatUint(uint64(status.GetClients()), 10))
					return nil
				},
			},
			{
				Name:  "stop",
				Usage: "stop the daemon; the next command that needs it starts it again",
				Action: func(c *cli.Context) error {
					// Find the state directory.
					dir, err := daemon.Dir()
					if err != nil {
						return err
					}

					// Stop its daemon, if one runs.
					running, err := daemon.Stop(c.Context, dir)
					if err == nil && !running {
						out.note("no modlock daemon is running")
					}
					return err
				},
			},
		},
	}
}

// connectDaemon connects to this PC's daemon of this release, starting it
// from this executable when needed.
func connectDaemon(ctx context.Context) (*daemon.Client, error) {
	// Find the state directory and this executable.
	dir, err := daemon.Dir()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "find the modlock executable")
	}

	// Connect, starting the daemon with its log beside its state.
	return daemon.Connect(ctx, dir, Version, func(ctx context.Context) error {
		return daemon.Start(ctx, exe, []string{"daemon"}, filepath.Join(dir, "daemon.log"))
	})
}
