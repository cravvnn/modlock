package main

import (
	"io"
	"slices"

	"github.com/aperturerobotics/cli"
	"github.com/paralin/modlock/peernet"
	"github.com/paralin/modlock/sandbox"
	"github.com/paralin/modlock/session"
	"github.com/pkg/errors"
)

// hostCommand runs a mod like modlock dev --watch and serves its server to
// guests on other PCs through this PC's peer.
func hostCommand(out *printer) *cli.Command {
	return &cli.Command{
		Name:      "host",
		Usage:     "run the mod like dev --watch and let players on other PCs join with modlock join",
		ArgsUsage: "[DIRECTORY]",
		Flags: append(
			slices.Clone(sessionFlags),
			&cli.UintFlag{
				Name:  "preview-port",
				Value: previewPort,
				Usage: "the loopback port of the page that draws the mod's interface; 0 turns it off",
			},
		),
		Action: func(c *cli.Context) error {
			return runDev(c, out, "host", true)
		},
	}
}

// hostSession serves running to guests until the command ends, printing the
// join code and each guest that joins or leaves. Hosting is optional: when it
// cannot start, the session keeps running for this PC alone.
func hostSession(c *cli.Context, out *printer, running server) {
	// Only a game server can take guests.
	if _, sandboxed := running.(*sandbox.Sandbox); sandboxed {
		out.mark("!", yellow, "guests join only a game server, not the sandbox")
		return
	}

	// Ask the daemon to host the server's port.
	client, err := connectDaemon(c.Context)
	if err != nil {
		out.mark("!", yellow, "hosting is off: "+err.Error())
		return
	}
	defer client.Close()
	stream, err := peernet.NewSRPCPeerNetClient(client.RPC()).Host(c.Context, &peernet.HostRequest{Port: uint32(c.Uint("port"))})
	if err != nil {
		out.mark("!", yellow, "hosting is off: "+err.Error())
		return
	}

	// Print each change until the session ends.
	for {
		event, err := stream.Recv()
		if err != nil {
			if c.Context.Err() == nil {
				out.mark("!", yellow, "hosting stopped: "+err.Error())
			}
			return
		}
		switch body := event.GetBody().(type) {
		case *peernet.HostEvent_JoinCode:
			out.field("join code", body.JoinCode)
			out.note("on the other PC: modlock join " + body.JoinCode)
		case *peernet.HostEvent_Joined:
			out.mark("+", green, "a guest joined: "+body.Joined)
		case *peernet.HostEvent_Left:
			out.mark("-", dim, "a guest left: "+body.Left)
		}
	}
}

// joinCommand joins a session another PC hosts and launches Deadlock into it.
func joinCommand(out *printer) *cli.Command {
	return &cli.Command{
		Name:      "join",
		Usage:     "join a session another PC hosts with modlock host, and launch Deadlock into it",
		ArgsUsage: "CODE",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "no-game", Usage: "only serve the session; join it yourself with the console line"},
		},
		Action: func(c *cli.Context) error {
			// Read the code and ask the daemon to join.
			code := c.Args().First()
			if code == "" {
				return errors.New("pass the join code modlock host printed")
			}
			out.header("join", code)
			client, err := connectDaemon(c.Context)
			if err != nil {
				return err
			}
			defer client.Close()
			stream, err := peernet.NewSRPCPeerNetClient(client.RPC()).Join(c.Context, &peernet.JoinRequest{Code: code})
			if err != nil {
				return err
			}

			// Launch the game at the endpoint, and hold until it leaves.
			out.mark("…", dim, "linking to the host")
			for {
				event, err := stream.Recv()
				if err != nil {
					if c.Context.Err() != nil || errors.Is(err, io.EOF) {
						return nil
					}
					return err
				}
				switch body := event.GetBody().(type) {
				case *peernet.JoinEvent_Address:
					launchGuest(c, out, body.Address)
				case *peernet.JoinEvent_Arrived:
					out.mark("✓", green, "in the host's game")
				}
			}
		},
	}
}

// launchGuest names the console line that joins address and launches
// Deadlock into it unless the guest joins by hand.
func launchGuest(c *cli.Context, out *printer, address string) {
	// Steam cannot connect a game that is already running, so name the line.
	out.field("join", "connect "+address+" in a running game's console")
	if c.Bool("no-game") {
		return
	}

	// Launch the game, which connects once it starts.
	steam, err := session.FindSteam()
	if err == nil {
		err = steam.LaunchDeadlock(c.Context, "-console", "-condebug", "+connect", address)
	}
	if err != nil {
		out.mark("!", yellow, "join from the console: "+err.Error())
	}
}
