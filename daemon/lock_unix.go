//go:build !windows

package daemon

import (
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// lockFile opens path and locks it exclusively. With wait false it returns
// errLocked at once when another process holds the lock. Closing the file
// releases the lock, and so does the process exiting.
func lockFile(path string, wait bool) (*os.File, error) {
	// Open the lock file, creating it once.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	// Take the lock, waiting for it only when asked.
	how := unix.LOCK_EX
	if !wait {
		how |= unix.LOCK_NB
	}
	for {
		err = unix.Flock(int(f.Fd()), how)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, errLocked
		}
		return nil, err
	}
	return f, nil
}
