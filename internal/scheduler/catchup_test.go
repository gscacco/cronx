package scheduler_test

import (
	"context"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

// catchUpJob builds a helper job that asks for the runs it missed while the
// scheduler was not running.
func catchUpJob(t *testing.T, name, mode, schedule string) job.Job {
	t.Helper()
	definition := clockJob(t, name, mode, schedule)
	definition.CatchUp = true
	return definition
}

// seedRun records a run that is already history, so that a test can say when
// cronx last looked at a job. The instant is the one the run started at, which
// is what the scheduler counts the activations a job missed from.
func seedRun(t *testing.T, persistent *store.Store, name string, at time.Time) {
	t.Helper()

	ctx := context.Background()
	id, err := persistent.StartRun(ctx, name, 1, at)
	if err != nil {
		t.Fatalf("recording a past run of job %q: %v", name, err)
	}
	code := 0
	if err := persistent.FinishRun(ctx, id, store.Finish{
		Status:     job.StatusSucceeded,
		FinishedAt: at.Add(time.Second),
		Duration:   time.Second,
		ExitCode:   &code,
	}); err != nil {
		t.Fatalf("closing the past run of job %q: %v", name, err)
	}
}

func TestAJobThatAsksIsCaughtUpWhenTheSchedulerStarts(t *testing.T) {
	// SETUP: a job that last ran two days before the activation it missed, and
	// a scheduler that is started after it.
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	subject, persistent, _ := buildSchedulerWithClock(t, clock.Fixed{T: now},
		catchUpJob(t, "backup", "ok", "0 3 * * *"))
	seedRun(t, persistent, "backup", now.AddDate(0, 0, -2))

	// EXERCISE
	runAndStop(t, subject)

	// VERIFY: the activation cronx was not there for was made up for, and the
	// history holds the run next to the one before it.
	runs := historyOf(t, persistent, "backup")
	if len(runs) != 2 {
		t.Fatalf("the job has %d runs, want 2: the one before and the one that was caught up", len(runs))
	}
	if runs[0].Status != job.StatusSucceeded {
		t.Errorf("the run that was caught up is %q, want %q", runs[0].Status, job.StatusSucceeded)
	}
}

func TestAJobThatDoesNotAskIsNotCaughtUp(t *testing.T) {
	// SETUP: the same job, without the option.
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	subject, persistent, _ := buildSchedulerWithClock(t, clock.Fixed{T: now},
		clockJob(t, "backup", "ok", "0 3 * * *"))
	seedRun(t, persistent, "backup", now.AddDate(0, 0, -2))

	// EXERCISE
	runAndStop(t, subject)

	// VERIFY: an activation that passed while the scheduler was not running is
	// skipped, as it always was.
	if runs := historyOf(t, persistent, "backup"); len(runs) != 1 {
		t.Errorf("the job has %d runs, want only the one it had", len(runs))
	}
}

func TestTheCatchUpIsOneRunHoweverManyActivationsWereMissed(t *testing.T) {
	// SETUP: a job that last ran forty days before the scheduler started
	// again, so that it missed forty activations of its own.
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	subject, persistent, _ := buildSchedulerWithClock(t, clock.Fixed{T: now},
		catchUpJob(t, "backup", "ok", "0 3 * * *"))
	seedRun(t, persistent, "backup", now.AddDate(0, 0, -40))

	// EXERCISE
	runAndStop(t, subject)

	// VERIFY: starting again is not a burst of runs.
	if runs := historyOf(t, persistent, "backup"); len(runs) != 2 {
		t.Errorf("the job has %d runs, want 2: one run for the activations that were missed", len(runs))
	}
}

func TestAJobIsNotCaughtUpTwice(t *testing.T) {
	// SETUP: a job that was caught up once, and a scheduler started again
	// afterwards.
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	clk := clock.Fixed{T: now}
	persistent, runs := openTestState(t, clk)
	seedRun(t, persistent, "backup", now.AddDate(0, 0, -2))
	runAndStop(t, buildSchedulerOn(t, persistent, runs, clk, "", catchUpJob(t, "backup", "ok", "0 3 * * *")))

	// EXERCISE: the scheduler is started a second time.
	runAndStop(t, buildSchedulerOn(t, persistent, runs, clk, "", catchUpJob(t, "backup", "ok", "0 3 * * *")))

	// VERIFY: what was made up for is not made up for again: the run that was
	// caught up is the one the second start counts from.
	if recorded := historyOf(t, persistent, "backup"); len(recorded) != 2 {
		t.Errorf("the job has %d runs, want 2: the catch-up happened once", len(recorded))
	}
}

func TestAJobThatHasNeverRunIsNotCaughtUp(t *testing.T) {
	// SETUP: a job that asks, with no history at all.
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	subject, persistent, _ := buildSchedulerWithClock(t, clock.Fixed{T: now},
		catchUpJob(t, "backup", "ok", "0 3 * * *"))

	// EXERCISE
	runAndStop(t, subject)

	// VERIFY: there is no instant to count the activations it missed from, so
	// it waits for the next one instead.
	if runs := historyOf(t, persistent, "backup"); len(runs) != 0 {
		t.Errorf("a job that has never run was caught up %d times, want none", len(runs))
	}
}

func TestARebootJobIsNotCaughtUpOnTopOfRunningAtTheStart(t *testing.T) {
	// SETUP: a job that runs when the scheduler starts and asks to be caught
	// up as well.
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	subject, persistent, _ := buildSchedulerWithClock(t, clock.Fixed{T: now},
		catchUpJob(t, "cache-warmer", "ok", "@reboot"))
	seedRun(t, persistent, "cache-warmer", now.AddDate(0, 0, -2))

	// EXERCISE
	runAndStop(t, subject)

	// VERIFY: its schedule has no activation on the clock, so the start is the
	// only trigger it has, and it is run once.
	if runs := historyOf(t, persistent, "cache-warmer"); len(runs) != 2 {
		t.Errorf("the job has %d runs, want the one it had and the one the start triggered", len(runs))
	}
}

func TestAReloadDoesNotCatchUpAJob(t *testing.T) {
	// SETUP: a running scheduler, and a job that is not in its configuration
	// yet but whose activation has already passed.
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	subject, persistent, _ := buildSchedulerWithClock(t, clock.Fixed{T: now},
		clockJob(t, "cleanup", "ok", everyMinute))
	seedRun(t, persistent, "backup", now.AddDate(0, 0, -2))

	// EXERCISE: the reload adds the job, which asks to be caught up.
	added := reloadedJobs(map[string]string{"cleanup": everyMinute, "backup": "0 3 * * *"})
	definition := added["backup"]
	definition.CatchUp = true
	added["backup"] = definition
	if err := subject.Reload(config.Config{Jobs: added}); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY: a scheduler that is already running is not a start, so the run
	// waits for the next one — as a job that runs at startup does.
	if runs := historyOf(t, persistent, "backup"); len(runs) != 1 {
		t.Errorf("the job has %d runs, want only the one it had: a reload does not catch up", len(runs))
	}
}
