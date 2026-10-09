package scheduler_test

import (
	"context"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/job"
)

func TestNextReturnsTheNextActivation(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "ok")
	definition.Schedule = "0 3 * * *"
	subject, _, _ := buildScheduler(t, definition)
	after := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)

	// EXERCISE
	next, ok := subject.Next("backup", after)

	// VERIFY
	if !ok {
		t.Fatalf("Next() reported no activation")
	}
	if want := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("Next() = %s, want %s", next, want)
	}
}

func TestNextReportsNoActivationForAnUnknownJob(t *testing.T) {
	// SETUP
	subject, _, _ := buildScheduler(t, helperJob(t, "backup", "ok"))

	// EXERCISE
	_, ok := subject.Next("missing", time.Now())

	// VERIFY
	if ok {
		t.Errorf("Next() reported an activation for a job that does not exist")
	}
}

func TestDueSkipsActivationsMissedWhileTheSchedulerWasStopped(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "ok")
	definition.Schedule = "0 3 * * *"
	// The scheduler starts four days after the activations it missed. A job
	// that asks to be caught up is run once when the scheduler starts, which
	// is a step of its own: what is asked here is that planning through Due
	// never replays what was missed.
	restarted := time.Date(2026, 1, 5, 4, 0, 0, 0, time.UTC)
	subject, _, _ := buildSchedulerWithClock(t, clock.Fixed{T: restarted}, definition)

	// EXERCISE
	due := subject.Due(restarted)

	// VERIFY
	if len(due) != 0 {
		t.Fatalf("Due() returned %d activations, want none: missed runs are not replayed", len(due))
	}

	next, ok := subject.Next("backup", restarted)
	if !ok {
		t.Fatalf("Next() reported no activation")
	}
	if want := time.Date(2026, 1, 6, 3, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("Next() = %s, want %s", next, want)
	}
}

func TestDueReturnsEachActivationOnce(t *testing.T) {
	// SETUP
	definition := helperJob(t, "backup", "ok")
	definition.Schedule = "0 3 * * *"
	started := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
	subject, _, _ := buildSchedulerWithClock(t, clock.Fixed{T: started}, definition)
	later := time.Date(2026, 1, 1, 3, 30, 0, 0, time.UTC)

	// EXERCISE
	first := subject.Due(later)
	second := subject.Due(later)

	// VERIFY
	if len(first) != 1 {
		t.Fatalf("len(Due()) = %d, want 1", len(first))
	}
	if first[0].Job.Name != "backup" {
		t.Errorf("Activation.Job.Name = %q, want %q", first[0].Job.Name, "backup")
	}
	if want := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC); !first[0].At.Equal(want) {
		t.Errorf("Activation.At = %s, want the scheduled instant %s", first[0].At, want)
	}
	if len(second) != 0 {
		t.Errorf("len(Due()) on the second call = %d, want 0", len(second))
	}
}

func TestRunStopsWhenTheContextIsCancelled(t *testing.T) {
	// SETUP
	subject, _, _ := buildScheduler(t, helperJob(t, "backup", "ok"))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// EXERCISE
	started := time.Now()
	err := subject.Run(ctx)
	elapsed := time.Since(started)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Run() took %s, want it to stop as soon as the context is cancelled", elapsed)
	}
}

func TestRunInterruptsRunsLeftBehindByAPreviousStop(t *testing.T) {
	// SETUP
	subject, persistent, _ := buildScheduler(t, helperJob(t, "backup", "ok"))
	ctx := context.Background()
	if _, err := persistent.StartRun(ctx, "backup", 1, time.Now()); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// EXERCISE
	if err := subject.Run(runCtx); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}

	// VERIFY
	runs, err := persistent.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if runs[0].Status != job.StatusFailed {
		t.Errorf("status of the run left behind = %q, want %q", runs[0].Status, job.StatusFailed)
	}
}
