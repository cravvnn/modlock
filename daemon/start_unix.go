//go:build !windows

package daemon

import "syscall"

// detached starts the daemon in its own session, apart from the terminal.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
