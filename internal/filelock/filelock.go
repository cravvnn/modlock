// Package filelock locks files between processes. The operating system
// releases a lock when its holder exits, however it exits.
package filelock

import "github.com/pkg/errors"

// ErrLocked reports that another process holds a lock against the request.
var ErrLocked = errors.New("locked by another process")
