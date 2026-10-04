package integration_test

import (
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
)

// The timeout tests check the promise that a job which does not finish on its
// own is stopped, and that stopping it stops what it started.

func TestAJobThatExceedsItsTimeoutIsStopped(t *testing.T) {
	if !processesCanBeInspected() {
		t.Skip("this system cannot be asked whether a process is still running")
	}

	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("timeout.toml")

	// EXERCISE
	startedAt := time.Now()
	outcome := environment.runOnce(configPath, "overrun")
	elapsed := time.Since(startedAt)

	// VERIFY
	if outcome.code == 0 {
		t.Fatalf("run-once succeeded, want a failure for a job that was stopped\nstdout:\n%s", outcome.stdout)
	}
	if !strings.Contains(outcome.stdout, string(job.StatusTimedOut)) {
		t.Errorf("run-once printed %q, want it to report %q", outcome.stdout, job.StatusTimedOut)
	}
	// The job sleeps for two minutes: waiting for it is not an option.
	if elapsed > settleBudget {
		t.Errorf("run-once took %s, want it to give up on the job soon after its one second timeout", elapsed)
	}

	run := environment.latestRun("overrun")
	if run.Status != job.StatusTimedOut {
		t.Errorf("the run was recorded as %q, want %q", run.Status, job.StatusTimedOut)
	}
	if !strings.Contains(run.Error, "timeout") {
		t.Errorf("the run recorded the error %q, want it to explain the timeout", run.Error)
	}
	if run.Duration >= 2*time.Minute {
		t.Errorf("the run is recorded as having lasted %s, want the timeout to have ended it", run.Duration)
	}

	// The process is gone: nothing is left running behind the scheduler.
	report := environment.waitForReport(environment.path("overrun.json"), settleBudget)
	t.Cleanup(func() { removeProcess(report.PID) })
	environment.waitFor("the job to be stopped", settleBudget, func() bool {
		return !processAlive(report.PID)
	})
}

func TestStoppingAJobStopsWhatTheJobStarted(t *testing.T) {
	if !processesCanBeInspected() {
		t.Skip("this system cannot be asked whether a process is still running")
	}

	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("timeout.toml")

	// EXERCISE
	outcome := environment.runOnce(configPath, "family")
	if outcome.code == 0 {
		t.Fatalf("run-once succeeded, want a failure for a job that was stopped\nstdout:\n%s", outcome.stdout)
	}

	// VERIFY
	report := environment.waitForReport(environment.path("family.json"), settleBudget)
	child := environment.waitForReport(environment.path("child.json"), settleBudget)
	t.Cleanup(func() {
		removeProcess(report.PID)
		removeProcess(child.PID)
	})

	// The job runs in a process group of its own, and the process it started
	// belongs to that group: that is what lets one signal stop both.
	if report.PGID != report.PID {
		t.Errorf("the job runs in the process group %d, want a group of its own, %d", report.PGID, report.PID)
	}
	if child.PID == report.PID {
		t.Error("the child has the same identifier as the job, want a process of its own")
	}
	if child.PGID != report.PGID {
		t.Errorf("the child runs in the process group %d, want the group of the job, %d", child.PGID, report.PGID)
	}

	environment.waitFor("the job and the process it started to be stopped", settleBudget, func() bool {
		return !processAlive(report.PID) && !processAlive(child.PID)
	})
}

func TestAJobThatIgnoresTheRequestToStopIsKilled(t *testing.T) {
	if !processesCanBeInspected() {
		t.Skip("this system cannot be asked whether a process is still running")
	}

	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("timeout.toml")

	// EXERCISE
	startedAt := time.Now()
	outcome := environment.runOnce(configPath, "stubborn")
	elapsed := time.Since(startedAt)

	// VERIFY
	if outcome.code == 0 {
		t.Fatalf("run-once succeeded, want a failure for a job that was stopped\nstdout:\n%s", outcome.stdout)
	}
	if !strings.Contains(outcome.stdout, string(job.StatusTimedOut)) {
		t.Errorf("run-once printed %q, want it to report %q", outcome.stdout, job.StatusTimedOut)
	}

	// The job is asked to stop when its timeout expires, refuses, and is killed
	// once the grace period is over: a stop that takes at least the grace
	// period, and not a minute longer.
	if elapsed < 9*time.Second {
		t.Errorf("run-once took %s, want the grace period to be given to the job before it is killed", elapsed)
	}
	if elapsed > settleBudget {
		t.Errorf("run-once took %s, want the job to be killed once the grace period is over", elapsed)
	}

	report := environment.waitForReport(environment.path("stubborn.json"), settleBudget)
	t.Cleanup(func() { removeProcess(report.PID) })
	environment.waitFor("the job to be killed", settleBudget, func() bool {
		return !processAlive(report.PID)
	})
	if run := environment.latestRun("stubborn"); run.Status != job.StatusTimedOut {
		t.Errorf("the run was recorded as %q, want %q", run.Status, job.StatusTimedOut)
	}
}
