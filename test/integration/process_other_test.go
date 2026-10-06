//go:build !unix

package integration_test

// processesCanBeInspected reports whether the tests can ask whether a process
// exists.
func processesCanBeInspected() bool { return false }

// processAlive reports whether a process with the given identifier exists.
func processAlive(int) bool { return true }

// removeProcess kills a process, which this system cannot do.
func removeProcess(int) {}
