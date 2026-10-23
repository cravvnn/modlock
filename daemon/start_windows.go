//go:build windows

package daemon

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// detached starts the daemon without a console, apart from the terminal.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
		HideWindow:    true,
	}
}
