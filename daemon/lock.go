package daemon

import (
	"context"
	"os"
)

// lockFileContext waits for an exclusive lock on path until ctx ends. A lock
// that arrives after ctx ended is released at once.
func lockFileContext(ctx context.Context, path string) (*os.File, error) {
	// Wait for the lock in the background, since the wait cannot be canceled.
	type result struct {
		f   *os.File
		err error
	}
	ch := make(chan result, 1)
	go func() {
		f, err := lockFile(path, true)
		ch <- result{f, err}
	}()

	// Return the lock, or release a late one after ctx ends.
	select {
	case r := <-ch:
		return r.f, r.err
	case <-ctx.Done():
		go func() {
			if r := <-ch; r.f != nil {
				_ = r.f.Close()
			}
		}()
		return nil, context.Cause(ctx)
	}
}
