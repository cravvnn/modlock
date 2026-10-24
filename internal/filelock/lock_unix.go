//go:build !windows

package filelock

import (
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// Lock opens path and locks it, exclusively or shared with other shared
// holders. With wait false it returns ErrLocked at once when the lock is
// held against it. Closing the file releases the lock, and so does the
// process exiting.
func Lock(path string, exclusive, wait bool) (*os.File, error) {
	// Open the lock file, creating it once.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	// Take the lock in its mode, waiting for it only when asked.
	how := unix.LOCK_SH
	if exclusive {
		how = unix.LOCK_EX
	}
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
			return nil, ErrLocked
		}
		return nil, err
	}
	return f, nil
}
