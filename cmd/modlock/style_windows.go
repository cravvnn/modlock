package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableColor turns on the console's handling of color codes, which older
// Windows consoles leave off, and reports whether it is on.
func enableColor(f *os.File) bool {
	var mode uint32
	handle := windows.Handle(f.Fd())
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
