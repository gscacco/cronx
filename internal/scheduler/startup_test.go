package scheduler_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
)

// rebootJob builds a helper job that is triggered by the scheduler starting
// rather than by the clock.
func rebootJob(t *testing.T, name, mode string) job.Job {
	t.Helper()
	definition := helperJob(t, name, mode)
	definition.Schedule = "@reboot"
	return definition
}

// clockJob builds a helper job that is triggered by the clock, so that a test
// can tell the two kinds apart.
func clockJob(t *testing.T, name, mode, schedule string) job.Job {
	t.Helper()
	definition := helperJob(t, name, mode)
	definition.Schedule = schedule
	return definition
}

func TestStartupReturnsTheJobsThatRunWhenTheSchedulerStarts(t *testing.T) {
	// SETUP
	subject, _, _ := buildScheduler(t,
		rebootJob(t, "cache-warmer", "ok"),
		clockJob(t, "backup", "ok", "0 3 * * *"))
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// EXERCISE
	starting := subject.Startup(at)

	// VERIFY: the job that runs at startup, and only that one, at the instant
	// the scheduler started.
	if len(starting) != 1 {
		t.Fatalf("Startup() returned %d activations, want only the job that runs at startup",
			len(starting))
	}
	if starting[0].Job.Name != "cache-warmer" {
		t.Errorf("Startup() triggered %q, want %q", starting[0].Job.Name, "cache-warmer")
	}
	if !starting[0].At.Equal(at) {
		t.Errorf("Startup() activated at %s, want %s, the instant the scheduler started",
			starting[0].At, at)
	}
}

func TestARebootJobHasNoActivationOnTheClock(t *testing.T) {
	// SETUP
	subject, _, _ := buildScheduler(t, rebootJob(t, "cache-warmer", "ok"))

	// EXERCISE
	next, ok := subject.Next("cache-warmer", time.Now())

	// VERIFY
	if ok {
		t.Errorf("Next() = %s, want no activation: the job runs when the scheduler starts", next)
	}
}

func TestRunsAtStartupReportsTheJobsItHolds(t *testing.T) {
	// SETUP
	subject, _, _ := buildScheduler(t,
		rebootJob(t, "cache-warmer", "ok"),
		clockJob(t, "backup", "ok", "0 3 * * *"))

	// EXERCISE / VERIFY
	if !subject.RunsAtStartup("cache-warmer") {
		t.Errorf("RunsAtStartup(%q) is false, want true", "cache-warmer")
	}
	for _, name := range []string{"backup", "missing"} {
		if subject.RunsAtStartup(name) {
			t.Errorf("RunsAtStartup(%q) is true, want false", name)
		}
	}
}

func TestARebootJobRunsOnceWhenTheSchedulerStarts(t *testing.T) {
	// SETUP
	subject, persistent, _ := buildScheduler(t, rebootJob(t, "cache-warmer", "ok"))
	runCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	// EXERCISE
	if err := subject.Run(runCtx); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}

	// VERIFY: the start triggered the job once, and the run is recorded like
	// any other, even though no cron expression ever matched.
	runs, err := persistent.Runs(context.Background(), "cache-warmer", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(Runs()) = %d, want 1: a reboot job runs once, when the scheduler starts",
			len(runs))
	}
	if runs[0].Status != job.StatusSucceeded {
		t.Errorf("recorded status = %q, want %q", runs[0].Status, job.StatusSucceeded)
	}
}

func TestReloadReportsTheJobItMakesRunAtStartup(t *testing.T) {
	// SETUP: a scheduler running one job, which the reload replaces with a job
	// that runs when the scheduler starts.
	var reported bytes.Buffer
	subject := buildSchedulerWithLogger(t, loggerInto(t, &reported),
		clockJob(t, "backup", "ok", "0 3 * * *"))

	// EXERCISE
	reloaded := config.Config{Jobs: reloadedJobs(map[string]string{
		"backup":       everyMinute,
		"cache-warmer": "@reboot",
	})}
	if err := subject.Reload(reloaded); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY: the job is not run now — the scheduler it is added to has
	// already started — and the reload says so instead of leaving a person
	// waiting for a run that is never coming.
	if !strings.Contains(reported.String(), "cache-warmer") {
		t.Errorf("the reload reported %q, want it to name the job that will run at the next start",
			reported.String())
	}
}
