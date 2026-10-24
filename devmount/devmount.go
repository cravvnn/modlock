// Package devmount owns the package a development session mounts in a
// Deadlock installation: its directory, its block in gameinfo.gi, and the lock
// that tells a live session from one that ended without unmounting. Another
// program that mounts content in the same game, such as a game client that
// refuses to start beside conflicting packages, clears a stale mount through
// ClearStale instead of asking the player to edit gameinfo.gi.
package devmount

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/paralin/modlock/internal/filelock"
	"github.com/paralin/modlock/source2/gameinfo"
	"github.com/pkg/errors"
)

const (
	// Name names the session's block in gameinfo.gi.
	Name = "MODLOCK DEV"
	// Dir is the game-relative directory of the session's package.
	Dir = "citadel/modlock_dev"
	// lockName is the lock file in Dir that each live session holds shared.
	lockName = "session.lock"
)

// ErrRunning reports that a live development session holds the mount.
var ErrRunning = errors.New("a Modlock development session is running in this game")

// Path returns the session's package directory in the installation at gameDir.
func Path(gameDir string) string {
	return filepath.Join(gameDir, "game", filepath.FromSlash(Dir))
}

// Hold marks a session live in the installation at gameDir until the returned
// file closes or the process exits, however it exits. Hold before mounting
// and close after unmounting. Several sessions may hold at once.
func Hold(gameDir string) (*os.File, error) {
	dir := Path(gameDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return filelock.Lock(filepath.Join(dir, lockName), false, true)
}

// Mount adds the session's block to the installation's gameinfo.gi, replacing
// a block a session left that did not end cleanly.
func Mount(gameDir string) error {
	info := infoPath(gameDir)
	source, err := os.ReadFile(info)
	if err != nil {
		return err
	}
	mounted, err := gameinfo.Mount(source, Name, Dir)
	if err != nil {
		return err
	}
	return os.WriteFile(info, mounted, 0o644)
}

// Unmount removes the session's block from the installation's gameinfo.gi,
// as it reads now, keeping every change another program made since.
func Unmount(gameDir string) error {
	info := infoPath(gameDir)
	source, err := os.ReadFile(info)
	if err != nil {
		return err
	}
	unmounted, err := gameinfo.Unmount(source, Name)
	if err != nil || bytes.Equal(unmounted, source) {
		return err
	}
	return os.WriteFile(info, unmounted, 0o644)
}

// ClearStale unmounts a block that no live session holds, and returns
// ErrRunning when one does. A block without a lock file came from a version
// before the lock and counts as stale.
func ClearStale(gameDir string) error {
	// Leave an installation without the block untouched.
	source, err := os.ReadFile(infoPath(gameDir))
	if err != nil {
		return err
	}
	unmounted, err := gameinfo.Unmount(source, Name)
	if err != nil || bytes.Equal(unmounted, source) {
		return err
	}

	// Unmount only while no session holds the lock.
	dir := Path(gameDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	held, err := filelock.Lock(filepath.Join(dir, lockName), true, false)
	if err == filelock.ErrLocked {
		return ErrRunning
	}
	if err != nil {
		return err
	}
	defer held.Close()
	return Unmount(gameDir)
}

// infoPath returns the installation's gameinfo.gi.
func infoPath(gameDir string) string {
	return filepath.Join(gameDir, "game", "citadel", "gameinfo.gi")
}
