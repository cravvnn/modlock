package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/paralin/modlock/lfs"
	"github.com/pkg/errors"
)

// lfsAgentName is the git-lfs transfer agent modlock lfs setup configures.
const lfsAgentName = "modlock"

// lfsCommand stores a mod repository's Git LFS objects in this PC's Space.
func lfsCommand(out *printer) *cli.Command {
	return &cli.Command{
		Name:  "lfs",
		Usage: "keep this Git repository's LFS files, such as VPKs, in this PC's mod store",
		Subcommands: []*cli.Command{
			{
				Name:  "setup",
				Usage: "send this repository's LFS files to this PC's mod store instead of the Git remote",
				Action: func(c *cli.Context) error {
					// Point git-lfs at this executable's agent.
					exe, err := os.Executable()
					if err != nil {
						return err
					}
					if _, err := git(c.Context, "lfs", "install", "--local", "--skip-repo"); err != nil {
						return errors.Wrap(err, "install git-lfs in this repository")
					}

					// Run one agent per transfer: every upload writes the same
					// object, so agents in parallel would only retry each other.
					prefix := "lfs.customtransfer." + lfsAgentName + "."
					for _, kv := range [][2]string{
						{"lfs.standalonetransferagent", lfsAgentName},
						{prefix + "path", filepath.Clean(exe)},
						{prefix + "args", "lfs agent"},
						{prefix + "concurrent", "false"},
						{"lfs.sshtransfer", "never"},
					} {
						if _, err := git(c.Context, "config", "--local", kv[0], kv[1]); err != nil {
							return errors.Wrap(err, "set "+kv[0])
						}
					}
					out.mark("✓", green, "LFS files go to this PC's mod store; git push sends the remote only their pointers")
					return nil
				},
			},
			{
				Name:   "agent",
				Usage:  "serve git-lfs transfers on stdin and stdout (git-lfs runs it)",
				Hidden: true,
				Action: func(c *cli.Context) error {
					// git-lfs moves downloads into its own store, so they land
					// on the repository's filesystem.
					tmpDir, err := git(c.Context, "rev-parse", "--path-format=absolute", "--git-path", "lfs/tmp")
					if err != nil {
						return lfs.Refuse(os.Stdin, os.Stdout, err)
					}

					// Serve git-lfs through the daemon.
					client, err := connectDaemon(c.Context)
					if err != nil {
						return lfs.Refuse(os.Stdin, os.Stdout, err)
					}
					defer client.Close()
					return lfs.RunAgent(c.Context, client.RPC(), tmpDir, os.Stdin, os.Stdout)
				},
			},
		},
	}
}

// git runs git with args in the working directory and returns its output.
func git(ctx context.Context, args ...string) (string, error) {
	// Report git's own message when it fails.
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		return "", errors.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(stdout)), nil
}
