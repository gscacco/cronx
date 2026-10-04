//go:build unix

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup puts the job in its own process group, so that a signal can
// be delivered to the whole job, including any process it starts.
func setProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateGroup asks the whole process group to terminate with SIGTERM.
func terminateGroup(process *os.Process) {
	// Setpgid makes the job the leader of its own group, so its process id is
	// also the group id; a negative pid addresses the group.
	_ = syscall.Kill(-process.Pid, syscall.SIGTERM)
}

// killGroup kills the whole process group with SIGKILL.
func killGroup(process *os.Process) {
	_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
}
