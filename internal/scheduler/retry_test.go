package scheduler_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
)

func TestExecuteRetriesUntilTheJobSucceeds(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "fail-times")
	definition.Retry = 2
	definition.Env[helperCounter] = filepath.Join(t.TempDir(), "counter")
	definition.Env[helperFailFor] = "2"
	subject, persistent, _ := buildScheduler(t, definition)
	ctx := context.Background()

	// EXERCISE
	status, err := subject.Execute(ctx, "backup", time.Now())

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusSucceeded {
		t.Errorf("Execute() = %q, want %q once a retry succeeds", status, job.StatusSucceeded)
	}

	runs, err := persistent.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("len(Runs()) = %d, want one record per attempt", len(runs))
	}

	// The history is newest first: the successful third attempt comes first.
	want := []struct {
		status  job.Status
		attempt int
	}{
		{job.StatusSucceeded, 3},
		{job.StatusFailed, 2},
		{job.StatusFailed, 1},
	}
	for index, expected := range want {
		if runs[index].Attempt != expected.attempt {
			t.Errorf("Runs()[%d].Attempt = %d, want %d", index, runs[index].Attempt, expected.attempt)
		}
		if runs[index].Status != expected.status {
			t.Errorf("Runs()[%d].Status = %q, want %q", index, runs[index].Status, expected.status)
		}
	}
}

func TestExecuteStopsRetryingWhenTheAttemptsAreExhausted(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "fail")
	definition.Env[helperExit] = "1"
	definition.Retry = 1
	subject, persistent, _ := buildScheduler(t, definition)
	ctx := context.Background()

	// EXERCISE
	status, err := subject.Execute(ctx, "backup", time.Now())

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusFailed {
		t.Errorf("Execute() = %q, want %q", status, job.StatusFailed)
	}

	runs, err := persistent.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(Runs()) = %d, want one record for each of the two attempts", len(runs))
	}
}

func TestExecuteRecordsATimeout(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "sleep")
	definition.Env[helperSleep] = "30s"
	definition.Timeout = 200 * time.Millisecond
	subject, persistent, _ := buildScheduler(t, definition)

	// EXERCISE
	status, err := subject.Execute(context.Background(), "backup", time.Now())

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusTimedOut {
		t.Errorf("Execute() = %q, want %q", status, job.StatusTimedOut)
	}

	runs, err := persistent.Runs(context.Background(), "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if runs[0].Status != job.StatusTimedOut {
		t.Errorf("recorded status = %q, want %q", runs[0].Status, job.StatusTimedOut)
	}
}
