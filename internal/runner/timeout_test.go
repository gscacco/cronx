package runner_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/runner"
)

func TestRunTimesOutAndTerminatesTheProcess(t *testing.T) {
	// SETUP
	command := newHelperCommand(t, "sleep", "")
	command.Env[helperSleep] = "30s"
	command.Timeout = testTimeout
	command.GracePeriod = 5 * time.Second

	// EXERCISE
	started := time.Now()
	result, err := runner.New(clock.System{}).Run(context.Background(), command)
	elapsed := time.Since(started)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("TimedOut = false, want true")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Run() took %s, want the process to stop soon after the timeout", elapsed)
	}
}

func TestRunAsksTheProcessToStopBeforeKillingIt(t *testing.T) {
	// SETUP
	var stdout strings.Builder
	command := newHelperCommand(t, "trap-term", "")
	command.Timeout = testTimeout
	command.GracePeriod = 5 * time.Second
	command.Stdout = &stdout

	// EXERCISE
	result, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("TimedOut = false, want true")
	}
	if !strings.Contains(stdout.String(), "terminated") {
		t.Fatalf("stdout = %q, want the process to have received SIGTERM", stdout.String())
	}
}

func TestRunKillsTheProcessAfterTheGracePeriod(t *testing.T) {
	// SETUP
	command := newHelperCommand(t, "ignore-term", "")
	command.Env[helperSleep] = "30s"
	command.Timeout = testTimeout
	command.GracePeriod = testTimeout

	// EXERCISE
	started := time.Now()
	result, err := runner.New(clock.System{}).Run(context.Background(), command)
	elapsed := time.Since(started)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("TimedOut = false, want true")
	}
	if result.ExitCode != exitedBySignal {
		t.Errorf("ExitCode = %d, want %d for a process killed by a signal", result.ExitCode, exitedBySignal)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("Run() took %s, want the process to be killed once the grace period expires", elapsed)
	}
}

func TestRunStopsWhenTheContextIsCancelled(t *testing.T) {
	// SETUP
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := newHelperCommand(t, "sleep", "")
	command.Env[helperSleep] = "30s"
	command.GracePeriod = testTimeout

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// EXERCISE
	started := time.Now()
	result, err := runner.New(clock.System{}).Run(ctx, command)
	elapsed := time.Since(started)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if result.TimedOut {
		t.Errorf("TimedOut = true, want false: the context was cancelled, not the timeout")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Run() took %s, want it to stop when the context is cancelled", elapsed)
	}
}
