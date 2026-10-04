//go:build unix

// Process inspection needs signals, which only some systems have: the tests
// that ask whether a process is still running skip themselves where they
// cannot ask.
package integration_test

import "syscall"

// processesCanBeInspected reports whether the tests can ask whether a process
// exists.
func processesCanBeInspected() bool { return true }

// processAlive reports whether a process with the given identifier exists.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// removeProcess kills a process and everything that shares its process group,
// which is what a test does with a job it lost track of.
func removeProcess(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
