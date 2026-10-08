package runner_test

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/runner"
)

// testTimeout keeps the timing-based tests fast while remaining far longer than
// the work the helper performs when it is not asked to sleep.
const testTimeout = 300 * time.Millisecond

// exitedBySignal is the exit code reported for a process terminated by a
// signal, including one killed on timeout.
const exitedBySignal = -1

// resolveSymlinks resolves the symlinks in path so that directory comparisons
// work on systems where the temporary directory is itself a symlink.
func resolveSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolving symlinks in %s: %v", path, err)
	}
	return resolved
}

func TestRunCapturesStandardOutputAndError(t *testing.T) {
	// SETUP
	var stdout, stderr strings.Builder
	command := newHelperCommand(t, "print", "")
	command.Stdout = &stdout
	command.Stderr = &stderr

	// EXERCISE
	result, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.TimedOut {
		t.Errorf("TimedOut = true, want false")
	}
	if !strings.Contains(stdout.String(), "stdout-line") {
		t.Errorf("stdout = %q, want it to contain the process output", stdout.String())
	}
	if !strings.Contains(stderr.String(), "stderr-line") {
		t.Errorf("stderr = %q, want it to contain the process error output", stderr.String())
	}
}

func TestRunReportsTheExitCode(t *testing.T) {
	// SETUP
	command := newHelperCommand(t, "exit", "")
	command.Env[helperExit] = "3"

	// EXERCISE
	result, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if result.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", result.ExitCode)
	}
	if result.TimedOut {
		t.Errorf("TimedOut = true, want false")
	}
}

func TestRunRecordsTimestampsFromTheClock(t *testing.T) {
	// SETUP
	instant := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	command := newHelperCommand(t, "print", "")

	// EXERCISE
	result, err := runner.New(clock.Fixed{T: instant}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if !result.StartedAt.Equal(instant) {
		t.Errorf("StartedAt = %s, want %s", result.StartedAt, instant)
	}
	if !result.FinishedAt.Equal(instant) {
		t.Errorf("FinishedAt = %s, want %s", result.FinishedAt, instant)
	}
	if result.Duration != 0 {
		t.Errorf("Duration = %s, want 0 for a fixed clock", result.Duration)
	}
}

func TestRunReportsThePidOfTheStartedProcess(t *testing.T) {
	// SETUP
	output := helperOutFile(t)
	command := newHelperCommand(t, "record-pid", output)
	var reported int
	command.OnStart = func(pid int) { reported = pid }

	// EXERCISE
	if _, err := runner.New(clock.System{}).Run(context.Background(), command); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}

	// VERIFY
	if reported <= 0 {
		t.Fatalf("OnStart reported the pid %d, want the identifier of the started process", reported)
	}
	want, err := strconv.Atoi(strings.TrimSpace(readRecorded(t, output)))
	if err != nil {
		t.Fatalf("reading the pid the process recorded: %v", err)
	}
	if reported != want {
		t.Errorf("OnStart reported the pid %d, want %d, the process that ran", reported, want)
	}
}

func TestRunAppliesTheWorkingDirectory(t *testing.T) {
	// SETUP
	directory := t.TempDir()
	output := helperOutFile(t)
	command := newHelperCommand(t, "record-cwd", output)
	command.Dir = directory

	// EXERCISE
	_, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	want := resolveSymlinks(t, directory)
	if got := resolveSymlinks(t, readRecorded(t, output)); got != want {
		t.Errorf("working directory = %q, want %q", got, want)
	}
}

func TestRunNamesAMissingWorkingDirectory(t *testing.T) {
	// SETUP
	missing := filepath.Join(t.TempDir(), "gone")
	command := newHelperCommand(t, "print", "")
	command.Dir = missing

	// EXERCISE
	result, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err == nil {
		t.Fatalf("Run() succeeded, want an error for a working directory that does not exist")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("Run() error = %q, want it to name the missing working directory %q", err.Error(), missing)
	}
	if result.StartedAt.IsZero() {
		t.Errorf("StartedAt is the zero time, want the time the attempt was made")
	}
}

func TestRunReportsASpawnError(t *testing.T) {
	// SETUP
	command := runner.Command{Path: "/nonexistent/cronx-missing-executable"}

	// EXERCISE
	result, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err == nil {
		t.Fatalf("Run() succeeded, want an error for a command that cannot be started")
	}
	if !strings.Contains(err.Error(), "cronx-missing-executable") {
		t.Errorf("Run() error = %q, want it to mention the command", err.Error())
	}
	if result.StartedAt.IsZero() {
		t.Errorf("StartedAt is the zero time, want the time the attempt was made")
	}
}
