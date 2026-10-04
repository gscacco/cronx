//go:build !unix

package runner

import (
	"os"
	"os/exec"
)

// On platforms without Unix process groups a job cannot be signalled together
// with the processes it starts: only the job process itself is controlled.

// setProcessGroup is a no-op on these platforms.
func setProcessGroup(*exec.Cmd) {}

// terminateGroup asks the process to terminate.
func terminateGroup(process *os.Process) {
	_ = process.Kill()
}

// killGroup kills the process.
func killGroup(process *os.Process) {
	_ = process.Kill()
}
