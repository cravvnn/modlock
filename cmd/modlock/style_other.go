//go:build !windows

package main

import "os"

// enableColor reports that a terminal here handles color codes.
func enableColor(*os.File) bool {
	return true
}
