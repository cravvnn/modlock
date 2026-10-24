// Command extension-go serves the echo service, which answers each call with
// the host's shout service's answer to the same payload; the hold service,
// whose methods take and release the hold on its running build; the reach
// service, whose "show" shows an entity only to the player in the slot its
// payload names and whose "command" runs a server command; and the crash
// service, which panics and so stops the mod.
package main

import (
	"strconv"

	"github.com/paralin/modlock/mod"
)

// init registers the service before the host starts the mod.
func init() {
	mod.Serve("echo", func(method string, payload []byte) ([]byte, error) {
		return mod.CallService("shout", method, payload)
	})
	mod.Serve("hold", func(method string, _ []byte) ([]byte, error) {
		return nil, mod.HoldReload(method == "take")
	})
	mod.Serve("reach", func(method string, payload []byte) ([]byte, error) {
		if method == "command" {
			return nil, mod.ServerCommand("echo")
		}
		slot, err := strconv.Atoi(string(payload))
		if err != nil {
			return nil, err
		}
		return nil, mod.ShowEntityOnlyTo(0, []int32{int32(slot)})
	})
	mod.Serve("crash", func(string, []byte) ([]byte, error) {
		panic("crash")
	})
}

// main is required by Go and never runs; the host calls the service.
func main() {}
