package integration_test

import (
	"fmt"
	"strings"
	"testing"
)

// The singleton tests check the promise that one state is driven by one
// scheduler: a second `cronx run` on the same state is refused instead of
// scheduling the same jobs again, and the state is given back when the
// scheduler stops.

func TestASecondSchedulerIsRefusedAndTheStateIsGivenBack(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("heartbeat.toml")
	first := environment.startScheduler(configPath)

	// EXERCISE: a second scheduler is asked to drive the same state.
	second := environment.run("run", "--config", configPath)

	// VERIFY: it refuses, it says what is in the way, and it never schedules
	// anything.
	if second.code == 0 {
		t.Fatalf("the second scheduler exited with status 0, want a refusal\nstdout:\n%s\nstderr:\n%s",
			second.stdout, second.stderr)
	}
	if !strings.Contains(second.stderr, "another scheduler") {
		t.Errorf("the second scheduler printed %q on standard error, want it to explain that another one holds the state",
			second.stderr)
	}
	if log := environment.readFile(environment.schedulerLog()); strings.Count(log, schedulerStarted) != 1 {
		t.Errorf("the scheduler log records %d schedulers starting, want only the one that took the state",
			strings.Count(log, schedulerStarted))
	}
	if !first.running() {
		t.Errorf("the scheduler that held the state exited\nstdout:\n%s\nstderr:\n%s",
			first.stdout(), first.stderr())
	}

	// EXERCISE: the scheduler that holds the state stops.
	if code := first.stop(); code != 0 {
		t.Fatalf("the scheduler exited with status %d, want success\nstdout:\n%s\nstderr:\n%s",
			code, first.stdout(), first.stderr())
	}

	// VERIFY: the state is free at once, without waiting for a lease nobody
	// holds any more, so the next scheduler starts.
	next := environment.startScheduler(configPath)
	if !next.running() {
		t.Fatalf("the scheduler started after the first one stopped exited\nstdout:\n%s\nstderr:\n%s",
			next.stdout(), next.stderr())
	}
	if code := next.stop(); code != 0 {
		t.Errorf("the scheduler exited with status %d, want success", code)
	}
}

func TestStatusReportsTheSchedulerThatHoldsTheState(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("heartbeat.toml")

	// EXERCISE: nothing has driven the state yet.
	idle := environment.runOK("status", "--config", configPath)

	// VERIFY
	if !strings.Contains(idle.stdout, "no scheduler is running") {
		t.Errorf("status printed %q, want it to say that nothing is driving the state", idle.stdout)
	}

	// SETUP: a scheduler takes the state.
	scheduler := environment.startScheduler(configPath)

	// EXERCISE
	running := environment.runOK("status", "--config", configPath)

	// VERIFY: the reader is told that jobs are being scheduled, by whom, and
	// for how long the state is theirs.
	if !strings.Contains(running.stdout, "scheduler running since") {
		t.Errorf("status printed %q, want it to report the scheduler that holds the state", running.stdout)
	}
	if holder := fmt.Sprintf(":%d", scheduler.command.Process.Pid); !strings.Contains(running.stdout, holder) {
		t.Errorf("status printed %q, want it to name the process %s that holds the state",
			running.stdout, holder)
	}

	// EXERCISE: the scheduler stops.
	if code := scheduler.stop(); code != 0 {
		t.Fatalf("the scheduler exited with status %d, want success", code)
	}

	// VERIFY: the state is free again, and status says so.
	after := environment.runOK("status", "--config", configPath)
	if !strings.Contains(after.stdout, "no scheduler is running") {
		t.Errorf("status printed %q after the scheduler stopped, want it to say that nothing is driving the state",
			after.stdout)
	}
}
