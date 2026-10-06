package integration_test

import (
	"os"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/job"
)

// The scheduling tests run the real scheduler, which plans its activations from
// a cron expression and therefore works to the minute. They wait for what the
// job does instead of sleeping, which is what makes them both deterministic and
// slower than the rest: they are the only tests of this package that wait for a
// real minute boundary, so they skip themselves under `go test -short`.

func TestAScheduledJobRunsAndIsRecorded(t *testing.T) {
	// A scheduled run waits for a real minute boundary. It is the cost of
	// testing the real scheduler, so the fast suite (go test -short, which is
	// what make test runs) leaves it to the full one (make test-full).
	if testing.Short() {
		t.Skip("a scheduled job waits for a real minute boundary: run the full suite with make test-full")
	}
	t.Parallel()

	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("heartbeat.toml")
	scheduler := environment.startScheduler(configPath)

	// EXERCISE: wait for the run the schedule produces.
	run := environment.waitForRun("heartbeat", job.StatusSucceeded, activationBudget)

	// VERIFY
	if !scheduler.running() {
		t.Fatalf("the scheduler exited on its own\nstdout:\n%s\nstderr:\n%s",
			scheduler.stdout(), scheduler.stderr())
	}

	// The job produced its observable result.
	if content := environment.readFile(environment.path("heartbeat")); content != "ran" {
		t.Errorf("the job wrote %q, want %q", content, "ran")
	}

	// The run is recorded, with what it did.
	if run.Attempt != 1 {
		t.Errorf("the run was recorded as attempt %d, want the first attempt", run.Attempt)
	}
	if run.ExitCode == nil || *run.ExitCode != 0 {
		t.Errorf("the run recorded the exit code %v, want 0", run.ExitCode)
	}
	if run.Error != "" {
		t.Errorf("the successful run recorded the error %q, want none", run.Error)
	}
	if run.Duration <= 0 {
		t.Errorf("the run recorded a duration of %s, want a positive one", run.Duration)
	}
	if !run.FinishedAt.After(run.StartedAt) {
		t.Errorf("the run finished at %s and started at %s, want the opposite order",
			run.FinishedAt, run.StartedAt)
	}

	// The output of the run is kept in the log file the database points at:
	// the one file every run of the installation appends to.
	if run.LogPath == "" {
		t.Fatal("the run was recorded without a log file")
	}
	expectedLog := environment.runsLog()
	if run.LogPath != expectedLog {
		t.Errorf("the run logged to %q, want %q", run.LogPath, expectedLog)
	}
	if output := environment.readFile(run.LogPath); !strings.Contains(output, "the heartbeat ran") {
		t.Errorf("the log of the run holds %q, want the output of the job", output)
	}

	// The commands report the same thing while the scheduler is still running,
	// which is what a person looking at the machine would see.
	history := environment.runOK("history", "heartbeat", "--config", configPath)
	if row := historyRowForAttempt(t, history.stdout, "heartbeat", 1); row.status != "succeeded" {
		t.Errorf("history reports attempt 1 as %q, want %q", row.status, "succeeded")
	}
	status := environment.runOK("status", "--config", configPath)
	if !strings.Contains(status.stdout, "succeeded") {
		t.Errorf("status printed %q, want the outcome of the job", status.stdout)
	}

	// The scheduler recorded what it did in its own log too.
	if log := environment.readFile(environment.schedulerLog()); !strings.Contains(log, schedulerStarted) {
		t.Errorf("the scheduler log holds %q, want it to record that the scheduler started", log)
	}

	// EXERCISE: stop the scheduler the way a service manager would.
	code := scheduler.stop()

	// VERIFY
	if code != 0 {
		t.Errorf("the scheduler exited with status %d, want success", code)
	}
	if runs := environment.runs("heartbeat"); len(runs) != 1 {
		t.Errorf("%d runs are recorded after the stop, want only the one that ran", len(runs))
	}
	if finished := environment.latestRun("heartbeat"); finished.Status != job.StatusSucceeded {
		t.Errorf("the run that had finished is recorded as %q, want %q", finished.Status, job.StatusSucceeded)
	}
	if _, err := os.Stat(environment.statePath()); err != nil {
		t.Errorf("the state database is missing: %v", err)
	}
}

func TestAnOverlappingRunIsSkipped(t *testing.T) {
	// A trigger can only overlap a run that a real activation started, and the
	// activation after that one is a minute away: this is the slowest test of
	// the suite, and the fast one (go test -short) leaves it alone.
	if testing.Short() {
		t.Skip("the overlapping trigger arrives at a second real minute boundary: run the full suite with make test-full")
	}
	t.Parallel()

	// SETUP: the job takes several minutes, so every activation after the
	// first arrives while the first run is still going.
	environment := newEnvironment(t)
	configPath := environment.configure("overlap.toml")
	scheduler := environment.startScheduler(configPath)

	// EXERCISE: wait for the first run, then for the trigger that arrives
	// while it is still in progress.
	first := environment.waitForRun("tick", job.StatusRunning, activationBudget)
	skipped := environment.waitForRun("tick", job.StatusSkipped, overlapBudget)

	// VERIFY
	if skipped.ID <= first.ID {
		t.Errorf("the skipped trigger (run %d) was recorded before the run it overlaps (run %d)",
			skipped.ID, first.ID)
	}
	if skipped.Attempt != 0 {
		t.Errorf("the skipped trigger is recorded as attempt %d, want 0 because no process was attempted",
			skipped.Attempt)
	}
	if skipped.ExitCode != nil {
		t.Errorf("the skipped run recorded the exit code %d, want none because no process ran", *skipped.ExitCode)
	}

	// The job ran once: a second, concurrent execution would have left a
	// second line behind.
	if ticks := environment.lines(environment.path("ticks")); len(ticks) != 1 {
		t.Errorf("the job started %d times, want once: %v", len(ticks), ticks)
	}
	if count := environment.runningRuns("tick"); count != 1 {
		t.Errorf("%d runs of the job are in progress, want exactly the first one", count)
	}

	// What a person reading the history sees: the skipped trigger, with no
	// exit status because no process ran.
	history := environment.runOK("history", "tick", "--config", configPath)
	row := historyRowForAttempt(t, history.stdout, "tick", 0)
	if row.status != string(job.StatusSkipped) {
		t.Errorf("history reports the skipped trigger as %q, want %q", row.status, job.StatusSkipped)
	}
	if row.exit != "-" {
		t.Errorf("history reports the exit status of the skipped trigger as %q, want a dash", row.exit)
	}

	// The scheduler is still running and stops cleanly, stopping the job it was
	// running with it.
	if !scheduler.running() {
		t.Fatalf("the scheduler exited on its own\nstdout:\n%s\nstderr:\n%s",
			scheduler.stdout(), scheduler.stderr())
	}
	report := environment.waitForReport(environment.path("tick.json"), settleBudget)
	t.Cleanup(func() { removeProcess(report.PID) })
	if code := scheduler.stop(); code != 0 {
		t.Errorf("the scheduler exited with status %d, want success", code)
	}
	if !processesCanBeInspected() {
		return
	}
	environment.waitFor("the job to be stopped", settleBudget, func() bool {
		return !processAlive(report.PID)
	})
}
