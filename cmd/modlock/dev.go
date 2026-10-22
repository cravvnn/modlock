package main

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/paralin/modlock/js"
	"github.com/paralin/modlock/preview"
	"github.com/paralin/modlock/project"
	modcli "github.com/paralin/modlock/proto/modlock/cli"
	"github.com/paralin/modlock/sandbox"
	"github.com/pkg/errors"
)

// previewPort is the default loopback port of the preview page.
const previewPort = 4321

// devCommand runs a mod in a local server and, with --watch, reloads it on
// each save. It serves the preview page, which draws the mod's interface.
func devCommand(out *printer) *cli.Command {
	return &cli.Command{
		Name:      "dev",
		Usage:     "build the mod and run it in a local server, or in the sandbox without the game",
		ArgsUsage: "[DIRECTORY]",
		Flags: append(
			slices.Clone(sessionFlags),
			&cli.BoolFlag{
				Name:  "watch",
				Usage: "rebuild the mod and reload it on each save",
			},
			&cli.UintFlag{
				Name:  "preview-port",
				Value: previewPort,
				Usage: "the loopback port of the page that draws the mod's interface; 0 turns it off",
			},
		),
		Action: func(c *cli.Context) error {
			// Open the project.
			opened, err := openProject(c)
			if err != nil {
				return err
			}

			// Serve the preview page, which takes the player's input once
			// the mods run in the sandbox.
			watch := c.Bool("watch")
			var box atomic.Pointer[sandbox.Sandbox]
			if port := c.Uint("preview-port"); port != 0 {
				servePreview(c.Context, out, int(port), watch, func(ctx context.Context, in *modcli.Input) string {
					if box := box.Load(); box != nil {
						return deliver(ctx, box, in)
					}
					return "the preview passes input to the mods only in the sandbox, once they run"
				})
			}

			// Build now and, with --watch, after each save. The first good
			// build starts the server and each later one reloads the mod; a
			// failed build keeps the running one. Watch calls rebuild on one
			// goroutine.
			var running server
			started := make(chan error, 1)
			rebuild := func() {
				// Keep the running mod when the build fails.
				if err := out.build(c.Context, opened, project.Dev); err != nil {
					out.note("the build failed; fix it and save to try again")
					return
				}

				// Reload a running server, or start the first one.
				if running != nil {
					_ = running.Reload(c.Context, opened.Output())
					return
				}
				var err error
				running, err = startSession(c, out, []string{opened.Output()}, filepath.Join(opened.Output(), "server.log"))
				if sandboxed, ok := running.(*sandbox.Sandbox); ok {
					box.Store(sandboxed)
				}
				started <- err
			}

			// Build once, then after each save when watching.
			go func() {
				rebuild()
				if !watch {
					return
				}
				err := opened.Watch(c.Context, func() {
					out.note("rebuilding", opened.Manifest.GetSlug())
					rebuild()
				})
				if err != nil && c.Context.Err() == nil {
					out.note(errors.Wrap(err, "stopped watching").Error())
				}
			}()

			// Run the session once it starts.
			select {
			case <-c.Context.Done():
				return nil
			case err := <-started:
				if err != nil {
					return err
				}
			}
			defer running.Stop()
			return waitSession(c, out, running)
		},
	}
}

// servePreview serves the preview page on the loopback port until ctx ends,
// for a mod that rebuilds on each save when watch is true, passing its input
// to input. The page is optional: when it cannot start,
// the mod runs without it.
func servePreview(ctx context.Context, out *printer, port int, watch bool, input preview.Input) {
	// Find the page's files in the library.
	library, err := js.Library(ctx)
	if err != nil {
		out.note("the preview page is off:", err)
		return
	}
	page := preview.New(filepath.Join(library, js.Preview), watch, input)

	// Listen on loopback, and follow the command line's events.
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		out.note("the preview page is off:", err)
		return
	}
	out.watch = page.Observe
	server := &http.Server{Handler: page, ReadHeaderTimeout: 10 * time.Second}
	context.AfterFunc(ctx, func() { _ = server.Close() })
	go func() { _ = server.Serve(listener) }()
	out.note("the preview is on http://" + address + "/")
}
