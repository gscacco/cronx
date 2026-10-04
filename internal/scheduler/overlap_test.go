package scheduler_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/scheduler"
)

// overlapSleep is long enough to observe a run while it is still in progress.
const overlapSleep = 400 * time.Millisecond

// slack is the extra time allowed on top of a job's own duration before a
// timing assertion is considered to have failed. It keeps the tests robust on a
// machine under load.
const slack = 200 * time.Millisecond

// startInBackground runs a job and reports its status on the returned channel.
func startInBackground(t *testing.T, subject *scheduler.Scheduler, name string) <-chan job.Status {
	t.Helper()
	status := make(chan job.Status, 1)
	go func() {
		got, err := subject.Execute(context.Background(), name, time.Now())
		if err != nil {
			t.Errorf("Execute() returned an unexpected error: %v", err)
		}
		status <- got
	}()
	return status
}

// longRunningJob builds a job that stays in progress for the overlap tests.
func longRunningJob(t *testing.T) job.Job {
	t.Helper()
	definition := helperJob(t, "backup", "sleep")
	definition.Env[helperSleep] = overlapSleep.String()
	return definition
}

func TestExecuteSkipsWhenThePreviousRunIsInProgress(t *testing.T) {
	// SETUP
	definition := longRunningJob(t)
	definition.Overlap = job.OverlapSkip
	subject, persistent, _ := buildScheduler(t, definition)
	ctx := context.Background()

	first := startInBackground(t, subject, "backup")
	waitFor(t, "the first run to start", func() bool {
		running, err := persistent.RunningRuns(ctx, "backup")
		return err == nil && running == 1
	})

	// EXERCISE
	status, err := subject.Execute(ctx, "backup", time.Now())

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusSkipped {
		t.Errorf("Execute() = %q, want %q", status, job.StatusSkipped)
	}
	if got := <-first; got != job.StatusSucceeded {
		t.Errorf("the run in progress = %q, want %q", got, job.StatusSucceeded)
	}

	runs, err := persistent.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(Runs()) = %d, want 2", len(runs))
	}
	if runs[0].Status != job.StatusSkipped {
		t.Errorf("newest run status = %q, want %q", runs[0].Status, job.StatusSkipped)
	}
	if !strings.Contains(runs[0].Error, "overlap") {
		t.Errorf("skipped run error = %q, want it to explain the overlap policy", runs[0].Error)
	}
}

func TestExecuteWaitsWhenTheOverlapPolicyIsQueue(t *testing.T) {
	// SETUP
	definition := longRunningJob(t)
	definition.Overlap = job.OverlapQueue
	subject, persistent, _ := buildScheduler(t, definition)
	ctx := context.Background()

	first := startInBackground(t, subject, "backup")
	waitFor(t, "the first run to start", func() bool {
		running, err := persistent.RunningRuns(ctx, "backup")
		return err == nil && running == 1
	})

	// EXERCISE
	started := time.Now()
	status, err := subject.Execute(ctx, "backup", time.Now())
	elapsed := time.Since(started)

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusSucceeded {
		t.Errorf("Execute() = %q, want %q", status, job.StatusSucceeded)
	}
	if elapsed < overlapSleep+slack {
		t.Errorf("the queued run finished after %s, want it to have waited for the run in progress", elapsed)
	}
	if got := <-first; got != job.StatusSucceeded {
		t.Errorf("the first run = %q, want %q", got, job.StatusSucceeded)
	}

	runs, err := persistent.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	for index, run := range runs {
		if run.Status != job.StatusSucceeded {
			t.Errorf("Runs()[%d].Status = %q, want %q", index, run.Status, job.StatusSucceeded)
		}
	}
}

func TestExecuteAllowsOverlapWhenTheOverlapPolicyIsAllow(t *testing.T) {
	// SETUP
	definition := longRunningJob(t)
	definition.Overlap = job.OverlapAllow
	subject, persistent, _ := buildScheduler(t, definition)
	ctx := context.Background()

	first := startInBackground(t, subject, "backup")
	waitFor(t, "the first run to start", func() bool {
		running, err := persistent.RunningRuns(ctx, "backup")
		return err == nil && running == 1
	})

	// EXERCISE
	started := time.Now()
	status, err := subject.Execute(ctx, "backup", time.Now())
	elapsed := time.Since(started)

	// VERIFY
	if err != nil {
		t.Fatalf("Execute() returned an unexpected error: %v", err)
	}
	if status != job.StatusSucceeded {
		t.Errorf("Execute() = %q, want %q", status, job.StatusSucceeded)
	}
	if elapsed > overlapSleep+slack {
		t.Errorf("the second run took %s, want it to run alongside the first", elapsed)
	}
	if got := <-first; got != job.StatusSucceeded {
		t.Errorf("the first run = %q, want %q", got, job.StatusSucceeded)
	}
}
