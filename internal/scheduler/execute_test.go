package scheduler_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
)

func TestExecuteRunsAJobAndRecordsSuccess(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "ok")
	subject, persistent, layout := buildScheduler(t, definition)
	trigger := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)

	// EXERCISE
	status, err := subject.Execute(context.Background(), "backup", trigger)

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusSucceeded {
		t.Errorf("Execute() = %q, want %q", status, job.StatusSucceeded)
	}

	runs, err := persistent.Runs(context.Background(), "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(Runs()) = %d, want 1", len(runs))
	}
	recorded := runs[0]
	if recorded.Status != job.StatusSucceeded {
		t.Errorf("recorded status = %q, want %q", recorded.Status, job.StatusSucceeded)
	}
	if recorded.Attempt != 1 {
		t.Errorf("recorded attempt = %d, want 1", recorded.Attempt)
	}
	if recorded.ExitCode == nil || *recorded.ExitCode != 0 {
		t.Errorf("recorded exit code = %v, want 0", recorded.ExitCode)
	}
	if want := layout.RunPath("backup", recorded.ID); recorded.LogPath != want {
		t.Errorf("recorded log path = %q, want %q", recorded.LogPath, want)
	}
}

func TestExecuteCapturesTheJobOutput(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "print")
	subject, persistent, layout := buildScheduler(t, definition)
	ctx := context.Background()

	// EXERCISE
	if _, err := subject.Execute(ctx, "backup", time.Now()); err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}

	// VERIFY
	runs, err := persistent.Runs(ctx, "backup", 1)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	content, err := os.ReadFile(layout.RunPath("backup", runs[0].ID))
	if err != nil {
		t.Fatalf("reading the run log: %v", err)
	}
	if !strings.Contains(string(content), "job output") {
		t.Errorf("run log = %q, want it to contain the output of the job", string(content))
	}
}

func TestExecuteRecordsAFailureWithItsExitCode(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "fail")
	definition.Env[helperExit] = "3"
	subject, persistent, _ := buildScheduler(t, definition)

	// EXERCISE
	status, err := subject.Execute(context.Background(), "backup", time.Now())

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusFailed {
		t.Errorf("Execute() = %q, want %q", status, job.StatusFailed)
	}

	runs, err := persistent.Runs(context.Background(), "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if runs[0].ExitCode == nil || *runs[0].ExitCode != 3 {
		t.Errorf("recorded exit code = %v, want 3", runs[0].ExitCode)
	}
}

func TestExecuteRecordsASpawnError(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "ok")
	definition.Command = "/nonexistent/cronx-missing-executable"
	subject, persistent, _ := buildScheduler(t, definition)

	// EXERCISE
	status, err := subject.Execute(context.Background(), "backup", time.Now())

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusSpawnError {
		t.Errorf("Execute() = %q, want %q", status, job.StatusSpawnError)
	}

	runs, err := persistent.Runs(context.Background(), "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if runs[0].Error == "" {
		t.Errorf("recorded error is empty, want the reason the job could not start")
	}
}

func TestExecuteRejectsAnUnknownJob(t *testing.T) {
	// SETUP
	subject, _, _ := buildScheduler(t, helperJob(t, "backup", "ok"))

	// EXERCISE
	_, err := subject.Execute(context.Background(), "missing", time.Now())

	// VERIFY
	if err == nil {
		t.Fatalf("Execute() succeeded, want an error for an unknown job")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("Execute() error = %q, want it to mention the job", err.Error())
	}
}
