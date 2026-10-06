//go:build !unix

package main

// processGroup returns the process group the program belongs to. This system
// has no process groups, so there is nothing to report.
func processGroup() int {
	return 0
}
