//go:build unix

package runner_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/runner"
)

// recordProcessGroup returns the process group id of the current process, for
// the helper process to record.
func recordProcessGroup() (string, error) {
	group, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		return "", err
	}
	return strconv.Itoa(group), nil
}

// TestRunPutsTheJobInItsOwnProcessGroup verifies that a job is isolated in a
// process group of its own. That isolation is what lets cronx stop a job
// together with the processes it starts, instead of leaving them behind.
func TestRunPutsTheJobInItsOwnProcessGroup(t *testing.T) {
	// SETUP
	output := helperOutFile(t)
	command := newHelperCommand(t, "record-pgid", output)

	// EXERCISE
	if _, err := runner.New(clock.System{}).Run(context.Background(), command); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}

	// VERIFY
	recorded := strings.TrimSpace(readRecorded(t, output))
	group, err := strconv.Atoi(recorded)
	if err != nil {
		t.Fatalf("parsing the recorded process group %q: %v", recorded, err)
	}
	if group == syscall.Getpgrp() {
		t.Errorf("job process group = %d, want a group of its own, not the scheduler's", group)
	}
}
