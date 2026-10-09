package scheduler_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/scheduler"
	"gscacco.com/cronx/internal/store"
)

// disabledJob builds a helper job the configuration keeps without scheduling.
func disabledJob(t *testing.T, name, mode, schedule string) job.Job {
	t.Helper()
	definition := clockJob(t, name, mode, schedule)
	definition.Disabled = true
	return definition
}

// disabledIn builds a job set for a reload in which the named job is disabled.
func disabledIn(name string, schedules map[string]string) map[string]job.Job {
	jobs := reloadedJobs(schedules)
	definition := jobs[name]
	definition.Disabled = true
	jobs[name] = definition
	return jobs
}

func TestADisabledJobHasNoActivationAndIsNotRunAtStartup(t *testing.T) {
	// SETUP
	subject, _, _ := buildScheduler(t, disabledJob(t, "backup", "ok", "0 3 * * *"))
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// EXERCISE
	next, scheduled := subject.Next("backup", now)

	// VERIFY: the job is held — `cronx list` still names it — but it has no
	// activation of its own and no trigger of any kind.
	if !subject.Disabled("backup") {
		t.Errorf("Disabled(%q) is false, want true", "backup")
	}
	if scheduled {
		t.Errorf("Next() = %s, want no activation: the job is disabled", next)
	}
	if subject.RunsAtStartup("backup") {
		t.Errorf("RunsAtStartup(%q) is true, want false: a disabled job is not triggered", "backup")
	}
	if starting := subject.Startup(now); len(starting) != 0 {
		t.Errorf("Startup() returned %d activations, want none: the job is disabled", len(starting))
	}
}

func TestDisabledReportsOnlyTheJobsItHolds(t *testing.T) {
	// SETUP: one disabled job, one that is scheduled, and a name that is not
	// configured at all.
	subject, _, _ := buildScheduler(t,
		disabledJob(t, "archiver", "ok", everyMinute),
		clockJob(t, "backup", "ok", everyMinute))

	// EXERCISE / VERIFY
	if !subject.Disabled("archiver") {
		t.Errorf("Disabled(%q) is false, want true", "archiver")
	}
	for _, name := range []string{"backup", "missing"} {
		if subject.Disabled(name) {
			t.Errorf("Disabled(%q) is true, want false", name)
		}
	}
}

func TestADisabledJobIsNeverRun(t *testing.T) {
	// SETUP: a scheduler whose only job is disabled. It is due every minute,
	// so a scheduler that scheduled it would run it within the test.
	definition := disabledJob(t, "backup", "ok", everyMinute)
	subject, persistent, _ := buildScheduler(t, definition)

	// EXERCISE
	runAndStop(t, subject)

	// VERIFY: the scheduler started and held the job without triggering it.
	if runs := historyOf(t, persistent, "backup"); len(runs) != 0 {
		t.Errorf("the disabled job has %d runs, want none", len(runs))
	}
}

func TestADisabledJobIsNotCaughtUpEither(t *testing.T) {
	// SETUP: a job that asks to be caught up and that the configuration
	// disables, with a history saying its activation has passed.
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	definition := catchUpJob(t, "backup", "ok", "0 3 * * *")
	definition.Disabled = true
	subject, persistent, _ := buildSchedulerWithClock(t, clock.Fixed{T: now}, definition)
	seedRun(t, persistent, "backup", now.AddDate(0, 0, -2))

	// EXERCISE
	runAndStop(t, subject)

	// VERIFY: a job that is not scheduled is not caught up either.
	if runs := historyOf(t, persistent, "backup"); len(runs) != 1 {
		t.Errorf("the disabled job has %d runs, want only the one it had", len(runs))
	}
}

func TestAReloadSchedulesAJobAgainWhenItIsEnabled(t *testing.T) {
	// SETUP: a scheduler running one job, which the reload keeps without
	// scheduling.
	subject := buildSchedulerWithLogger(t, loggerInto(t, &bytes.Buffer{}),
		clockJob(t, "backup", "ok", everyMinute))

	// EXERCISE
	if err := subject.Reload(config.Config{
		Jobs: disabledIn("backup", map[string]string{"backup": everyMinute}),
	}); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY: the job is held without an activation.
	if !subject.Disabled("backup") {
		t.Fatalf("Disabled(%q) is false, want true: the reload disabled the job", "backup")
	}
	if next, scheduled := subject.Next("backup", time.Now()); scheduled {
		t.Errorf("Next() = %s, want no activation after the reload disabled the job", next)
	}

	// EXERCISE: a second reload enables it again.
	if err := subject.Reload(config.Config{
		Jobs: reloadedJobs(map[string]string{"backup": everyMinute}),
	}); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY: it is scheduled again, from the moment of the reload.
	if subject.Disabled("backup") {
		t.Fatalf("Disabled(%q) is true, want false: the reload enabled the job", "backup")
	}
	if _, scheduled := subject.Next("backup", time.Now()); !scheduled {
		t.Errorf("Next() reported no activation, want the job to be scheduled again")
	}
}

func TestReloadReportsTheJobItDisables(t *testing.T) {
	// SETUP: a scheduler running one job.
	var reported bytes.Buffer
	subject := buildSchedulerWithLogger(t, loggerInto(t, &reported),
		clockJob(t, "backup", "ok", everyMinute))

	// EXERCISE
	if err := subject.Reload(config.Config{
		Jobs: disabledIn("backup", map[string]string{"backup": everyMinute}),
	}); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY: a job that stops being triggered is explained, because nothing
	// else would say why it stopped running.
	if !strings.Contains(reported.String(), "backup") {
		t.Errorf("the reload reported %q, want it to name the job it disabled", reported.String())
	}
	if !strings.Contains(reported.String(), "disabled") {
		t.Errorf("the reload reported %q, want it to say that the job is not scheduled", reported.String())
	}
}

// runAndStop runs a scheduler until what it triggered at the start has run, and
// then stops it. It returns once the scheduler has finished, so that a test can
// read the history it left behind.
func runAndStop(t *testing.T, subject *scheduler.Scheduler) {
	t.Helper()

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- subject.Run(runCtx) }()

	time.Sleep(300 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
}

// historyOf returns the recorded runs of a job, newest first.
func historyOf(t *testing.T, persistent *store.Store, name string) []store.Run {
	t.Helper()
	runs, err := persistent.Runs(context.Background(), name, 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	return runs
}
