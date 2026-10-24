package main

import (
	"context"
	"io"
	"slices"
	"strconv"
	"sync"

	"github.com/aperturerobotics/cli"
	modcli "github.com/paralin/modlock/cli"
	"github.com/paralin/modlock/peernet"
	"github.com/paralin/modlock/sandbox"
	"github.com/paralin/modlock/session"
	"github.com/paralin/modlock/src/host_app/control"
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
// join code and each guest that joins or leaves, and sends the daemon each
// player's interface from feed. Hosting is optional: when it cannot start,
// the session keeps running for this PC alone.
func hostSession(c *cli.Context, out *printer, running server, feed *hostFeed) {
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
	stream, err := peernet.NewSRPCPeerNetClient(client.RPC()).Host(c.Context)
	if err == nil {
		start := &peernet.HostRequest{Port: uint32(c.Uint("port"))}
		err = stream.Send(&peernet.HostInput{Body: &peernet.HostInput_Start{Start: start}})
	}
	if err != nil {
		out.mark("!", yellow, "hosting is off: "+err.Error())
		return
	}
	go feed.send(c.Context, stream)

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

// hostFeed keeps every player's interface from the session's first event, so
// a daemon that starts hosting later gets the whole state, then each change.
type hostFeed struct {
	// mu guards the fields below.
	mu sync.Mutex
	// screens holds every player's interface.
	screens *session.Screens
	// queue holds the events the daemon has yet to receive, once sending.
	queue []*control.HostEvent
	// sending is true once send took the state.
	sending bool
	// wake holds a token while queue may have events.
	wake chan struct{}
}

// newHostFeed returns a feed with no players.
func newHostFeed() *hostFeed {
	return &hostFeed{screens: session.NewScreens(0), wake: make(chan struct{}, 1)}
}

// observe follows the players and interfaces in event.
func (f *hostFeed) observe(event *modcli.Event) {
	// Take only the events that change a player's screen.
	host := event.GetHost()
	if host.GetJoined() == nil && host.GetLeft() == nil && host.GetUi() == nil {
		return
	}

	// Keep the state, and queue the change once sending.
	f.mu.Lock()
	defer f.mu.Unlock()
	f.screens.Handle(host)
	if f.sending {
		f.queue = append(f.queue, host)
		f.signal()
	}
}

// signal wakes send. Callers hold mu.
func (f *hostFeed) signal() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

// send sends the daemon the whole state, then each change, until ctx ends or
// the stream fails.
func (f *hostFeed) send(ctx context.Context, stream peernet.SRPCPeerNet_HostClient) {
	// Start from the state so far.
	f.mu.Lock()
	f.queue, f.sending = f.screens.Snapshot(), true
	f.signal()
	f.mu.Unlock()
	defer f.stop()

	// Send each batch as it arrives.
	for {
		select {
		case <-ctx.Done():
			return
		case <-f.wake:
		}
		f.mu.Lock()
		batch := f.queue
		f.queue = nil
		f.mu.Unlock()
		for _, event := range batch {
			if stream.Send(&peernet.HostInput{Body: &peernet.HostInput_Server{Server: event}}) != nil {
				return
			}
		}
	}
}

// stop drops the queue once nothing sends it.
func (f *hostFeed) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue, f.sending = nil, false
}

// joinCommand joins a session another PC hosts and launches Deadlock into it.
func joinCommand(out *printer) *cli.Command {
	return &cli.Command{
		Name:      "join",
		Usage:     "join a session another PC hosts with modlock host, and launch Deadlock into it",
		ArgsUsage: "CODE",
		Flags: append([]cli.Flag{
			&cli.BoolFlag{Name: "no-game", Usage: "only serve the session; join it yourself with the console line"},
		}, interfaceFlags...),
		Action: func(c *cli.Context) error {
			// Read the code.
			code := c.Args().First()
			if code == "" {
				return errors.New("pass the join code modlock host printed")
			}
			out.header("join", code)

			// Serve this player's interfaces before the game starts.
			mirror := session.NewMirror()
			account, stop := serveGuestInterface(c, out, mirror)
			defer stop()

			// Ask the daemon to join, streaming this player's screen.
			client, err := connectDaemon(c.Context)
			if err != nil {
				return err
			}
			defer client.Close()
			stream, err := peernet.NewSRPCPeerNetClient(client.RPC()).Join(c.Context, &peernet.JoinRequest{Code: code, SteamId: account})
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
				case *peernet.JoinEvent_Screen:
					mirror.Apply(body.Screen.GetChanges(), body.Screen.GetReset_())
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

// serveGuestInterface serves mirror to the game and mounts the renderer, and
// returns the player's Steam ID and the call that stops serving. Without
// Steam or with the interfaces off it serves nothing and returns zero, so the
// host sends no screen.
func serveGuestInterface(c *cli.Context, out *printer, mirror *session.Mirror) (uint64, func()) {
	// Find the player and their game.
	port := c.Uint("ui-port")
	steam, err := session.FindSteam()
	if port == 0 || err != nil || steam.Account() == 0 {
		return 0, func() {}
	}
	gameDir, err := steam.Deadlock()
	if err != nil {
		out.mark("!", yellow, "mod interfaces are off: "+err.Error())
		return 0, func() {}
	}

	// Serve the screen through the relay page.
	renderer, err := readRenderer(c.Context)
	if err != nil {
		out.mark("!", yellow, "mod interfaces are off: "+err.Error())
		return 0, func() {}
	}
	stop := session.StartInterface(session.Interface{
		Address:  "127.0.0.1:" + strconv.Itoa(int(port)),
		Relay:    c.String("ui-relay"),
		Renderer: renderer,
		GameDir:  gameDir,
		Screen:   mirror.Screen,
		Events:   out.host,
	})
	return steam.Account(), stop
}
