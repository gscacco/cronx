package store_test

import (
	"context"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

func TestStartRunRecordsARunningRun(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	started := mustTime(t, "2026-01-01T03:00:00Z")

	// EXERCISE
	id, err := subject.StartRun(ctx, "backup", 2, started)

	// VERIFY
	if err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if id <= 0 {
		t.Errorf("StartRun() id = %d, want a positive identifier", id)
	}

	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(Runs()) = %d, want 1", len(runs))
	}
	got := runs[0]
	if got.ID != id {
		t.Errorf("Run.ID = %d, want %d", got.ID, id)
	}
	if got.Job != "backup" {
		t.Errorf("Run.Job = %q, want %q", got.Job, "backup")
	}
	if got.Attempt != 2 {
		t.Errorf("Run.Attempt = %d, want 2", got.Attempt)
	}
	if got.Status != job.StatusRunning {
		t.Errorf("Run.Status = %q, want %q", got.Status, job.StatusRunning)
	}
	if !got.StartedAt.Equal(started) {
		t.Errorf("Run.StartedAt = %s, want %s", got.StartedAt, started)
	}
	if !got.FinishedAt.IsZero() {
		t.Errorf("Run.FinishedAt = %s, want the zero time while the run is in progress", got.FinishedAt)
	}
	if got.ExitCode != nil {
		t.Errorf("Run.ExitCode = %d, want none while the run is in progress", *got.ExitCode)
	}
}

func TestFinishRunRecordsASuccessfulOutcome(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	started := mustTime(t, "2026-01-01T03:00:00Z")
	finished := started.Add(2 * time.Minute)
	const logPath = "/home/user/.cronx/logs/backup/1.log"

	id, err := subject.StartRun(ctx, "backup", 1, started)
	if err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}

	// EXERCISE
	err = subject.FinishRun(ctx, id, store.Finish{
		Status:     job.StatusSucceeded,
		FinishedAt: finished,
		Duration:   2 * time.Minute,
		ExitCode:   intPointer(0),
		LogPath:    logPath,
	})

	// VERIFY
	if err != nil {
		t.Fatalf("FinishRun() returned an unexpected error: %v", err)
	}

	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	got := runs[0]
	if got.Status != job.StatusSucceeded {
		t.Errorf("Run.Status = %q, want %q", got.Status, job.StatusSucceeded)
	}
	if !got.FinishedAt.Equal(finished) {
		t.Errorf("Run.FinishedAt = %s, want %s", got.FinishedAt, finished)
	}
	if got.Duration != 2*time.Minute {
		t.Errorf("Run.Duration = %s, want %s", got.Duration, 2*time.Minute)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("Run.ExitCode = %v, want 0", got.ExitCode)
	}
	if got.LogPath != logPath {
		t.Errorf("Run.LogPath = %q, want %q", got.LogPath, logPath)
	}
}

func TestFinishRunRecordsAFailureWithItsMessage(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	const failure = "starting /usr/local/bin/backup: no such file or directory"

	id, err := subject.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T03:00:00Z"))
	if err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}

	// EXERCISE
	err = subject.FinishRun(ctx, id, store.Finish{
		Status:     job.StatusSpawnError,
		FinishedAt: mustTime(t, "2026-01-01T03:00:01Z"),
		Duration:   time.Second,
		Error:      failure,
	})

	// VERIFY
	if err != nil {
		t.Fatalf("FinishRun() returned an unexpected error: %v", err)
	}

	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	got := runs[0]
	if got.Status != job.StatusSpawnError {
		t.Errorf("Run.Status = %q, want %q", got.Status, job.StatusSpawnError)
	}
	if got.Error != failure {
		t.Errorf("Run.Error = %q, want %q", got.Error, failure)
	}
}

func TestRunningRunsCountsOnlyRunsInProgress(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	first, err := subject.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T03:00:00Z"))
	if err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if _, err := subject.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T03:01:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if _, err := subject.StartRun(ctx, "other", 1, mustTime(t, "2026-01-01T03:02:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if err := subject.FinishRun(ctx, first, store.Finish{
		Status:     job.StatusSucceeded,
		FinishedAt: mustTime(t, "2026-01-01T03:03:00Z"),
	}); err != nil {
		t.Fatalf("FinishRun() returned an unexpected error: %v", err)
	}

	// EXERCISE
	running, err := subject.RunningRuns(ctx, "backup")

	// VERIFY
	if err != nil {
		t.Fatalf("RunningRuns() returned an unexpected error: %v", err)
	}
	if running != 1 {
		t.Errorf("RunningRuns(backup) = %d, want 1", running)
	}
}

func TestRecordSkippedAddsARunWithoutAnExitCode(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	at := mustTime(t, "2026-01-01T03:00:00Z")

	// EXERCISE
	err := subject.RecordSkipped(ctx, "backup", at, "overlap policy is skip")

	// VERIFY
	if err != nil {
		t.Fatalf("RecordSkipped() returned an unexpected error: %v", err)
	}

	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(Runs()) = %d, want 1", len(runs))
	}
	got := runs[0]
	if got.Status != job.StatusSkipped {
		t.Errorf("Run.Status = %q, want %q", got.Status, job.StatusSkipped)
	}
	if !got.StartedAt.Equal(at) {
		t.Errorf("Run.StartedAt = %s, want %s", got.StartedAt, at)
	}
	if !got.FinishedAt.Equal(at) {
		t.Errorf("Run.FinishedAt = %s, want %s", got.FinishedAt, at)
	}
	if got.ExitCode != nil {
		t.Errorf("Run.ExitCode = %d, want none for a skipped run", *got.ExitCode)
	}
}
