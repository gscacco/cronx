package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/job"
)

// The scheduling tests run the real scheduler, which plans its activations from
// a cron expression and therefore works to the minute. They wait for what the
// job does instead of sleeping, which is what makes them both deterministic and
// slower than the rest.

func TestAScheduledJobRunsAndIsRecorded(t *testing.T) {
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

	// The output of the run is kept in the log file the database points at.
	if run.LogPath == "" {
		t.Fatal("the run was recorded without a log file")
	}
	expectedLog := filepath.Join(environment.jobLogDirectory("heartbeat"), "1.log")
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
	if log := environment.readFile(environment.schedulerLog()); !strings.Contains(log, "scheduler started") {
		t.Errorf("the scheduler log holds %q, want it to record that the scheduler started", log)
	}

	// EXERCISE: stop the scheduler the way a service manager would.
	code := scheduler.stop()

	// VERIFY
	if code != 0 {
		t.Errorf("the scheduler exited with status %d, want success", code)
	}
	if count := environment.runningRuns("heartbeat"); count != 0 {
		t.Errorf("%d runs are still recorded as in progress, want none after a clean stop", count)
	}
	if _, err := os.Stat(environment.statePath()); err != nil {
		t.Errorf("the state database is missing: %v", err)
	}
}
