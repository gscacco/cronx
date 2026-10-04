//go:build unix

package main

import (
	"os"
	"syscall"
)

// processGroup returns the process group the program belongs to, or zero when
// it cannot be determined.
func processGroup() int {
	group, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		return 0
	}
	return group
}
