// Package daemon runs one modlock process per state directory that owns the
// creator's store and peer, and connects the other modlock processes to it.
//
// The daemon holds an exclusive lock on its directory's daemon.lock for its
// whole life and serves starpc over a local socket beside it. A client holds
// start.lock while it connects, and starts a detached daemon when none answers,
// waiting for its ready line. A daemon started by hand takes start.lock while
// it binds, one that Start launched relies on its starter holding it, and idle
// shutdown takes it too, so a client sees either a daemon that is listening or
// no daemon at all, never one that is closing.
package daemon

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	// SocketName is the daemon's socket within its state directory.
	SocketName = "modlock.sock"
	// lockName is the file the running daemon holds locked.
	lockName = "daemon.lock"
	// startName is the file held locked while a daemon starts or stops.
	startName = "start.lock"
	// readyLine is what a starting daemon writes to its standard output once
	// it listens.
	readyLine = "ready\n"
	// StateEnv names the environment variable that overrides the state
	// directory.
	StateEnv = "MODLOCK_STATE"
	// startedEnv marks a daemon that Start launched, whose starter holds
	// start.lock.
	startedEnv = "MODLOCK_DAEMON_STARTED"
)

var (
	// ErrRunning reports that another daemon already holds the state
	// directory.
	ErrRunning = errors.New("a modlock daemon already runs for this state directory")
	// errLocked reports that another process holds a lock file.
	errLocked = errors.New("locked by another process")
)

// Dir returns the state directory: $MODLOCK_STATE, or modlock/state in the
// user's configuration directory.
func Dir() (string, error) {
	// Prefer the explicit directory.
	if dir := os.Getenv(StateEnv); dir != "" {
		return filepath.Abs(dir)
	}

	// Default to the user's configuration directory.
	config, err := os.UserConfigDir()
	if err != nil {
		return "", errors.Wrap(err, "find the configuration directory")
	}
	return filepath.Join(config, "modlock", "state"), nil
}
