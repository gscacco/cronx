package integration_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/job"
)

// The retry tests check that a failing job is attempted again as many times as
// the configuration asks, and that every attempt is visible in the history.

func TestAFailingJobIsRetried(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("retry.toml")

	// EXERCISE
	outcome := environment.runOnce(configPath, "failing")

	// VERIFY
	if outcome.code == 0 {
		t.Fatalf("run-once succeeded, want a failure for a job that never succeeds\nstdout:\n%s", outcome.stdout)
	}
	if !strings.Contains(outcome.stdout, string(job.StatusFailed)) {
		t.Errorf("run-once printed %q, want it to report %q", outcome.stdout, job.StatusFailed)
	}

	// One trigger, one attempt plus the two the configuration asks for.
	runs := environment.runs("failing")
	if len(runs) != 3 {
		t.Fatalf("the job was attempted %d times, want 3 because retry is 2\n%s", len(runs), environment.describe())
	}
	for index, wanted := range []int{3, 2, 1} {
		run := runs[index]
		if run.Attempt != wanted {
			t.Errorf("attempt %d was recorded as attempt %d, want %d", index+1, run.Attempt, wanted)
		}
		if run.Status != job.StatusFailed {
			t.Errorf("attempt %d was recorded as %q, want %q", wanted, run.Status, job.StatusFailed)
		}
		if run.ExitCode == nil || *run.ExitCode != 3 {
			t.Errorf("attempt %d recorded the exit code %v, want the 3 the job exits with", wanted, run.ExitCode)
		}
		if run.LogPath == "" {
			t.Errorf("attempt %d was recorded without a log file, want every attempt to keep its output", wanted)
		}
	}
	if runs[2].ID >= runs[1].ID || runs[1].ID >= runs[0].ID {
		t.Errorf("the attempts were recorded as runs %d, %d and %d, want an increasing identifier",
			runs[2].ID, runs[1].ID, runs[0].ID)
	}

	// The history shows one row per attempt, and the status shows the outcome
	// of the last one.
	history := environment.runOK("history", "failing", "--config", configPath)
	if rows := historyRows(t, history.stdout, "failing"); len(rows) != 3 {
		t.Errorf("history lists %d attempts, want 3\n%s", len(rows), history.stdout)
	}
	for _, attempt := range []int{1, 2, 3} {
		row := historyRowForAttempt(t, history.stdout, "failing", attempt)
		if row.status != string(job.StatusFailed) {
			t.Errorf("history reports attempt %d as %q, want %q", attempt, row.status, job.StatusFailed)
		}
		if row.exit != "3" {
			t.Errorf("history reports the exit status of attempt %d as %q, want 3", attempt, row.exit)
		}
	}
	status := environment.runOK("status", "--config", configPath)
	if !strings.Contains(status.stdout, string(job.StatusFailed)) {
		t.Errorf("status printed %q, want the final outcome of the job", status.stdout)
	}
	if count := environment.runningRuns("failing"); count != 0 {
		t.Errorf("%d runs are recorded as in progress, want none once the job gave up", count)
	}
}

func TestARetryThatSucceedsStopsTheAttempts(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("retry.toml")

	// EXERCISE
	outcome := environment.runOnce(configPath, "eventually")

	// VERIFY: the third attempt succeeds, so run-once succeeds too.
	if outcome.code != 0 {
		t.Fatalf("run-once exited with status %d, want success once an attempt succeeded\nstdout:\n%s\nstderr:\n%s",
			outcome.code, outcome.stdout, outcome.stderr)
	}
	if !strings.Contains(outcome.stdout, string(job.StatusSucceeded)) {
		t.Errorf("run-once printed %q, want it to report %q", outcome.stdout, job.StatusSucceeded)
	}

	// Two failures and then a success: the attempts stop there, even though
	// the configuration allows one more.
	runs := environment.runs("eventually")
	if len(runs) != 3 {
		t.Fatalf("the job was attempted %d times, want 3: two failures and the success\n%s",
			len(runs), environment.describe())
	}
	wanted := []job.Status{job.StatusSucceeded, job.StatusFailed, job.StatusFailed}
	for index, status := range wanted {
		if runs[index].Status != status {
			t.Errorf("run %d was recorded as %q, want %q", runs[index].ID, runs[index].Status, status)
		}
	}
	if runs[0].ExitCode == nil || *runs[0].ExitCode != 0 {
		t.Errorf("the successful attempt recorded the exit code %v, want 0", runs[0].ExitCode)
	}
	for _, failed := range runs[1:] {
		if failed.ExitCode == nil || *failed.ExitCode != 3 {
			t.Errorf("run %d recorded the exit code %v, want the 3 the job exits with", failed.ID, failed.ExitCode)
		}
	}
	if count := environment.runningRuns("eventually"); count != 0 {
		t.Errorf("%d runs are recorded as in progress, want none once the job succeeded", count)
	}
}
