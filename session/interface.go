package session

import (
	"net"
	"net/http"

	"github.com/paralin/modlock/src/host_app/control"
	"github.com/pkg/errors"
)

// Interface configures serving a player's mod interfaces to their game.
type Interface struct {
	// Address is the loopback address the relay page reads from.
	Address string
	// Relay is the relay page's origin.
	Relay string
	// Renderer is the built renderer script; nil leaves the game without it,
	// so the interfaces show only in the browser preview.
	Renderer []byte
	// GameDir is the Deadlock installation the renderer mounts in.
	GameDir string
	// Screen is the player's screen.
	Screen ScreenSource
	// Events receives a failure that turns part of the interfaces off.
	Events func(*control.HostEvent)
}

// StartInterface serves the screen to the relay page and mounts the renderer
// in the game, and returns the call that stops both. Each part is optional:
// another program on the address or a game that cannot take the renderer
// reports a failure and leaves the session running. Start it before the game
// launches, which loads the renderer only at its start.
func StartInterface(config Interface) (stop func()) {
	// Serve the screen. Without it the renderer has nothing to draw.
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		config.fail(errors.Wrap(err, "mod interfaces are off"))
		return func() {}
	}
	server := &http.Server{Handler: NewRelay(config.Relay, config.Screen)}
	go func() { _ = server.Serve(listener) }()
	if config.Renderer == nil {
		return func() { _ = server.Close() }
	}

	// Mount the renderer, so the game draws the screen.
	unmount, err := mountRenderer(config.GameDir, config.Renderer)
	if err != nil {
		config.fail(errors.Wrap(err, "mod interfaces show only in the browser preview"))
		return func() { _ = server.Close() }
	}
	return func() {
		_ = server.Close()
		_ = unmount()
	}
}

// fail reports err as a failure event.
func (c Interface) fail(err error) {
	c.Events(&control.HostEvent{Body: &control.HostEvent_Failed{Failed: &control.ModFailed{Error: err.Error()}}})
}
