package store_test

import (
	"context"
	"testing"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

func TestRunsAreReturnedNewestFirstAndLimited(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	for _, started := range []string{
		"2026-01-01T03:00:00Z",
		"2026-01-02T03:00:00Z",
		"2026-01-03T03:00:00Z",
	} {
		if _, err := subject.StartRun(ctx, "backup", 1, mustTime(t, started)); err != nil {
			t.Fatalf("StartRun() returned an unexpected error: %v", err)
		}
	}

	// EXERCISE
	runs, err := subject.Runs(ctx, "backup", 2)

	// VERIFY
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(Runs()) = %d, want 2", len(runs))
	}
	if !runs[0].StartedAt.Equal(mustTime(t, "2026-01-03T03:00:00Z")) {
		t.Errorf("Runs()[0].StartedAt = %s, want the newest run first", runs[0].StartedAt)
	}
	if !runs[1].StartedAt.Equal(mustTime(t, "2026-01-02T03:00:00Z")) {
		t.Errorf("Runs()[1].StartedAt = %s, want the second newest run", runs[1].StartedAt)
	}
}

func TestRunsCanBeFilteredByJob(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	if _, err := subject.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T03:00:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if _, err := subject.StartRun(ctx, "cleanup", 1, mustTime(t, "2026-01-01T04:00:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}

	// EXERCISE
	runs, err := subject.Runs(ctx, "cleanup", 10)

	// VERIFY
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(Runs(cleanup)) = %d, want 1", len(runs))
	}
	if runs[0].Job != "cleanup" {
		t.Errorf("Run.Job = %q, want %q", runs[0].Job, "cleanup")
	}
}

func TestRunsWithoutAJobReturnEveryJob(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	if _, err := subject.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T03:00:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if _, err := subject.StartRun(ctx, "cleanup", 1, mustTime(t, "2026-01-01T04:00:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}

	// EXERCISE
	runs, err := subject.Runs(ctx, "", 10)

	// VERIFY
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(Runs(\"\")) = %d, want 2", len(runs))
	}
}

func TestInterruptStaleRunsMarksUnfinishedRunsAsFailed(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	if _, err := subject.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T03:00:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if _, err := subject.StartRun(ctx, "cleanup", 1, mustTime(t, "2026-01-01T04:00:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	restartedAt := mustTime(t, "2026-01-02T00:00:00Z")

	// EXERCISE
	interrupted, err := subject.InterruptStaleRuns(ctx, restartedAt)

	// VERIFY
	if err != nil {
		t.Fatalf("InterruptStaleRuns() returned an unexpected error: %v", err)
	}
	if interrupted != 2 {
		t.Errorf("InterruptStaleRuns() = %d, want 2", interrupted)
	}

	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	got := runs[0]
	if got.Status != job.StatusFailed {
		t.Errorf("Run.Status = %q, want %q", got.Status, job.StatusFailed)
	}
	if !got.FinishedAt.Equal(restartedAt) {
		t.Errorf("Run.FinishedAt = %s, want %s", got.FinishedAt, restartedAt)
	}
	if got.Error == "" {
		t.Errorf("Run.Error is empty, want an explanation that the scheduler stopped")
	}

	for _, name := range []string{"backup", "cleanup"} {
		running, err := subject.RunningRuns(ctx, name)
		if err != nil {
			t.Fatalf("RunningRuns(%s) returned an unexpected error: %v", name, err)
		}
		if running != 0 {
			t.Errorf("RunningRuns(%s) = %d, want 0 after interrupting stale runs", name, running)
		}
	}
}

func TestInterruptStaleRunsLeavesFinishedRunsAlone(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	id, err := subject.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T03:00:00Z"))
	if err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if err := subject.FinishRun(ctx, id, store.Finish{
		Status:     job.StatusSucceeded,
		FinishedAt: mustTime(t, "2026-01-01T03:01:00Z"),
	}); err != nil {
		t.Fatalf("FinishRun() returned an unexpected error: %v", err)
	}

	// EXERCISE
	interrupted, err := subject.InterruptStaleRuns(ctx, mustTime(t, "2026-01-02T00:00:00Z"))

	// VERIFY
	if err != nil {
		t.Fatalf("InterruptStaleRuns() returned an unexpected error: %v", err)
	}
	if interrupted != 0 {
		t.Errorf("InterruptStaleRuns() = %d, want 0", interrupted)
	}

	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if runs[0].Status != job.StatusSucceeded {
		t.Errorf("Run.Status = %q, want %q", runs[0].Status, job.StatusSucceeded)
	}
}
