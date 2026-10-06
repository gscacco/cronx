package cli_test

import (
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
)

const twoJobs = `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"

[jobs.cleanup]
schedule = "*/15 * * * *"
command = "/usr/local/bin/cleanup"
`

func TestListShowsTheConfiguredJobs(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)

	// EXERCISE
	output, err := runCLI(t, "list", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("list returned an unexpected error: %v", err)
	}
	for _, want := range []string{"JOB", "SCHEDULE", "NEXT", "backup", "cleanup", "0 3 * * *", "*/15 * * * *"} {
		if !strings.Contains(output, want) {
			t.Errorf("list output = %q, want it to contain %q", output, want)
		}
	}
}

func TestListRejectsAJobWhoseScheduleCannotBeParsed(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, `
[jobs.backup]
schedule = "not a cron expression"
command = "/usr/local/bin/backup"
`)

	// EXERCISE
	_, err := runCLI(t, "list", "--config", path)

	// VERIFY
	if err == nil {
		t.Fatalf("list succeeded, want an error for an unusable schedule")
	}
	if !strings.Contains(err.Error(), "backup") {
		t.Errorf("list error = %q, want it to name the job", err.Error())
	}
}

func TestListReportsWhenNoJobIsConfigured(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, "\n")

	// EXERCISE
	output, err := runCLI(t, "list", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("list returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "no jobs") {
		t.Errorf("list output = %q, want it to say that nothing is configured", output)
	}
}

func TestHistoryShowsRecordedRuns(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	started := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	seedRun(t, home, "backup", job.StatusSucceeded, started, 0)
	seedRun(t, home, "cleanup", job.StatusFailed, started.Add(time.Hour), 3)

	// EXERCISE
	output, err := runCLI(t, "history", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("history returned an unexpected error: %v", err)
	}
	for _, want := range []string{"ID", "ATTEMPT", "STATUS", "succeeded", "failed", "backup", "cleanup"} {
		if !strings.Contains(output, want) {
			t.Errorf("history output = %q, want it to contain %q", output, want)
		}
	}
}

func TestHistoryCanBeLimitedToASingleJob(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	started := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	seedRun(t, home, "backup", job.StatusSucceeded, started, 0)
	seedRun(t, home, "cleanup", job.StatusFailed, started.Add(time.Hour), 3)

	// EXERCISE
	output, err := runCLI(t, "history", "cleanup", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("history returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "cleanup") {
		t.Errorf("history output = %q, want it to contain the job", output)
	}
	if strings.Contains(output, "backup") {
		t.Errorf("history output = %q, want it to leave the other job out", output)
	}
}

func TestHistoryReportsAnEmptyHistory(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)

	// EXERCISE
	output, err := runCLI(t, "history", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("history returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "no runs") {
		t.Errorf("history output = %q, want it to say that nothing ran yet", output)
	}
}

func TestStatusShowsTheLastOutcomeOfEveryJob(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	started := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	seedRun(t, home, "backup", job.StatusSucceeded, started, 0)

	// EXERCISE
	output, err := runCLI(t, "status", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("status returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "succeeded") {
		t.Errorf("status output = %q, want the outcome of the job that ran", output)
	}
	if !strings.Contains(output, "never run") {
		t.Errorf("status output = %q, want the job that never ran to say so", output)
	}
	if !strings.Contains(output, "no") {
		t.Errorf("status output = %q, want it to report whether runs are in progress", output)
	}
}

func TestVersionPrintsTheVersion(t *testing.T) {
	// SETUP
	newTestHome(t)

	// EXERCISE
	output, err := runCLI(t, "version")

	// VERIFY
	if err != nil {
		t.Fatalf("version returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "0.1.0") {
		t.Errorf("version output = %q, want it to contain the version", output)
	}
}
