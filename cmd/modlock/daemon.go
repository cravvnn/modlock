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
	"github.com/paralin/modlock/lfs"
	"github.com/paralin/modlock/peernet"
	"github.com/paralin/modlock/publish"
	"github.com/paralin/modlock/space"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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
			&cli.StringFlag{Name: "origin", Value: publish.DefaultOrigin, Usage: "the Hyperline service that introduces this PC's peer to others"},
		},
		Action: func(c *cli.Context) error {
			// Find the state directory.
			dir, err := daemon.Dir()
			if err != nil {
				return err
			}

			// Register the PC's peer on the daemon's services.
			peer, err := newPeer(dir, c.String("origin"))
			if err != nil {
				return err
			}
			mux := srpc.NewMux()
			if err := peer.Register(mux); err != nil {
				return err
			}

			// Register the Space's LFS objects beside it.
			objects := lfs.NewServer()
			if err := objects.Register(mux); err != nil {
				return err
			}

			// Serve them, unless another daemon already does. The peer and
			// the Space open their files only once this daemon holds the
			// directory.
			ctx, cancel := context.WithCancel(c.Context)
			defer cancel()
			le := logrus.NewEntry(logrus.StandardLogger())
			err = daemon.Serve(ctx, dir, Version, c.Duration("idle"), mux, func() {
				daemon.Ready()
				go runPeer(ctx, le, peer)
				go runSpace(ctx, le, filepath.Join(dir, "space.s4db"), objects)
			})
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

					// Show its peer and where it stands with signaling.
					peer, err := peernet.NewSRPCPeerNetClient(client.RPC()).GetPeer(c.Context, &peernet.GetPeerRequest{})
					if err != nil {
						return err
					}
					out.field("peer", peer.GetPeerId())
					out.field("signaling", admissionText(peer))
					return nil
				},
			},
			{
				Name:      "probe",
				Usage:     "link this PC's peer to another and measure the round trip",
				ArgsUsage: "PEER",
				Action: func(c *cli.Context) error {
					// Read the other peer and connect to the daemon.
					remote := c.Args().First()
					if remote == "" {
						return errors.New("name the other PC's peer, which modlock daemon status shows there")
					}
					client, err := connectDaemon(c.Context)
					if err != nil {
						return err
					}
					defer client.Close()

					// Probe it and show the link.
					probe, err := peernet.NewSRPCPeerNetClient(client.RPC()).Probe(c.Context, &peernet.ProbeRequest{PeerId: remote})
					if err != nil {
						return err
					}
					out.field("round trip", (time.Duration(probe.GetRttUs()) * time.Microsecond).String())
					path := "direct"
					if probe.GetRelayed() {
						path = "relayed"
					}
					out.field("path", path)
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

// newPeer constructs the daemon's peer in dir, signaling at origin with the
// sign-in modlock publish saves.
func newPeer(dir, origin string) (*peernet.Service, error) {
	sessionFile, err := hyperlineSessionFile()
	if err != nil {
		return nil, err
	}
	return peernet.NewService(origin, filepath.Join(dir, "peer.pem"), func() peernet.SignIn {
		saved := publish.SavedSession(sessionFile, origin)
		return peernet.SignIn{Token: saved.GetToken(), Account: saved.GetName()}
	}), nil
}

// runPeer runs peer until ctx ends, logging why it stopped early. The daemon
// keeps serving its other services without it.
func runPeer(ctx context.Context, le *logrus.Entry, peer *peernet.Service) {
	if err := peer.Run(ctx, le); err != nil && ctx.Err() == nil {
		le.WithError(err).Warn("the peer stopped")
	}
}

// runSpace opens the Space at path and serves its LFS objects until ctx
// ends, logging why it stopped early. The daemon keeps serving its other
// services without it.
func runSpace(ctx context.Context, le *logrus.Entry, path string, objects *lfs.Server) {
	// Open the Space.
	sp, err := space.Open(ctx, le, path)
	if err != nil {
		objects.Fail(err)
	} else {
		defer sp.Release()
		err = objects.Serve(ctx, le, sp)
	}

	// Report a failure that was not the daemon stopping.
	if err != nil && ctx.Err() == nil {
		le.WithError(err).Warn("the mod store stopped")
	}
}

// admissionText says where peer stands with its signaling service.
func admissionText(peer *peernet.GetPeerResponse) string {
	switch peer.GetAdmission() {
	case peernet.AdmissionState_ADMISSION_STATE_ADMITTED:
		return "signed in as " + peer.GetAccount() + " at " + peer.GetOrigin()
	case peernet.AdmissionState_ADMISSION_STATE_CONNECTING:
		if peer.GetError() != "" {
			return "reconnecting to " + peer.GetOrigin() + ": " + peer.GetError()
		}
		return "connecting to " + peer.GetOrigin()
	case peernet.AdmissionState_ADMISSION_STATE_REFUSED:
		return peer.GetOrigin() + " refused the saved sign-in; modlock publish signs in again"
	default:
		return "signed out; modlock publish signs in to " + peer.GetOrigin()
	}
}
