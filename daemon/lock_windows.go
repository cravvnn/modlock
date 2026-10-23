//go:build windows

package daemon

import (
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
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

	// Take the lock over the file's first byte, waiting for it only when asked.
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	err = windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &windows.Overlapped{})
	if err != nil {
		_ = f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errLocked
		}
		return nil, err
	}
	return f, nil
}
