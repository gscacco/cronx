package scheduler_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
)

func TestExecuteRunsAJobAndRecordsSuccess(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "ok")
	subject, persistent, runLog := buildScheduler(t, definition)
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
	if want := runLog.Path(); recorded.LogPath != want {
		t.Errorf("recorded log path = %q, want the shared run log %q", recorded.LogPath, want)
	}
}

func TestExecuteCapturesTheJobOutput(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "print")
	subject, persistent, runLog := buildScheduler(t, definition)
	ctx := context.Background()

	// EXERCISE
	if _, err := subject.Execute(ctx, "backup", time.Now()); err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}

	// VERIFY
	recorded, err := persistent.Runs(ctx, "backup", 1)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	content, err := os.ReadFile(runLog.Path())
	if err != nil {
		t.Fatalf("reading the shared run log: %v", err)
	}
	prefix := "backup id=" + strconv.FormatInt(recorded[0].ID, 10) + " pid="
	if !strings.Contains(string(content), prefix) {
		t.Errorf("run log = %q, want every line to identify the run with %q", string(content), prefix)
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

func TestExecuteFinishesTheRunThatIsInFlightWhenTheSchedulerStops(t *testing.T) {
	// SETUP: a job that runs until it is stopped.
	definition := helperJob(t, "backup", "sleep")
	definition.Env[helperSleep] = "30s"
	subject, persistent, _ := buildScheduler(t, definition)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// EXERCISE: stop the scheduler while the job is in flight.
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_, _ = subject.Execute(ctx, "backup", time.Now())
	}()
	waitFor(t, "the run to be recorded as in progress", func() bool {
		running, err := persistent.RunningRuns(context.Background(), "backup")
		return err == nil && running == 1
	})
	cancel()
	<-stopped

	// VERIFY: the history is final as soon as the scheduler has stopped, so the
	// row does not keep saying "running" until the next start repairs it.
	runs, err := persistent.Runs(context.Background(), "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(Runs()) = %d, want the single run that was started", len(runs))
	}
	if runs[0].Status == job.StatusRunning {
		t.Errorf("the run is still %q after the scheduler stopped, want it finished", runs[0].Status)
	}
	if runs[0].FinishedAt.IsZero() {
		t.Error("the run has no finishing time, want the instant the scheduler stopped it")
	}
}
