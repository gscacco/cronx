package scheduler_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/logx"
	"gscacco.com/cronx/internal/runner"
	"gscacco.com/cronx/internal/scheduler"
)

// buildSchedulerWithLogger wires a scheduler whose logger writes what it does
// into the given buffer, so that a test can read what a reload reported.
func buildSchedulerWithLogger(t *testing.T, logger *slog.Logger, jobs ...job.Job) *scheduler.Scheduler {
	t.Helper()

	clk := clock.System{}
	persistent, runs := openTestState(t, clk)

	configured := make(map[string]job.Job, len(jobs))
	for _, definition := range jobs {
		configured[definition.Name] = definition
	}

	built, err := scheduler.New(scheduler.Options{
		Config: config.Config{
			Scheduler: config.Scheduler{Timezone: "Local", MaxParallelJobs: 4},
			Logging:   config.Logging{Level: "info"},
			Jobs:      configured,
		},
		Store:  persistent,
		Logs:   runs,
		Runner: runner.New(clk),
		Clock:  clk,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("building the scheduler: %v", err)
	}
	return built
}

// loggerInto returns a logger that writes into the given buffer.
func loggerInto(t *testing.T, buffer *bytes.Buffer) *slog.Logger {
	t.Helper()
	logger, err := logx.NewLogger(buffer, "info")
	if err != nil {
		t.Fatalf("building the logger: %v", err)
	}
	return logger
}

// reloadedJobs builds the job set of a reload from names and schedules.
func reloadedJobs(schedules map[string]string) map[string]job.Job {
	jobs := make(map[string]job.Job, len(schedules))
	for name, schedule := range schedules {
		jobs[name] = job.Job{Name: name, Schedule: schedule, Command: "/usr/local/bin/" + name}
	}
	return jobs
}

func TestReloadReplacesTheJobsTheSchedulerRuns(t *testing.T) {
	// SETUP: a scheduler running one job.
	backup := helperJob(t, "backup", "ok")
	backup.Schedule = "0 3 * * *"
	subject := buildSchedulerWithLogger(t, loggerInto(t, &bytes.Buffer{}), backup)
	after := time.Date(2026, 1, 1, 4, 0, 0, 0, time.UTC)

	// EXERCISE: the configuration now holds another job.
	reloaded := config.Config{Jobs: reloadedJobs(map[string]string{"cleanup": "0 5 * * *"})}
	if err := subject.Reload(reloaded); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY: the new job is scheduled, the one that is gone is not.
	next, ok := subject.Next("cleanup", after)
	if !ok {
		t.Fatalf("Next(cleanup) reported no activation, want the job the reload added")
	}
	if want := time.Date(2026, 1, 1, 5, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("Next(cleanup) = %s, want %s", next, want)
	}
	if _, ok := subject.Next("backup", after); ok {
		t.Errorf("Next(backup) reported an activation, want the job the reload removed to be gone")
	}
}

func TestReloadKeepsTheRunningConfigurationWhenTheNewOneIsUnusable(t *testing.T) {
	// SETUP: a scheduler running one job.
	backup := helperJob(t, "backup", "ok")
	backup.Schedule = "0 3 * * *"
	subject := buildSchedulerWithLogger(t, loggerInto(t, &bytes.Buffer{}), backup)

	// EXERCISE: the new configuration holds a schedule that cannot be read.
	reloaded := config.Config{Jobs: reloadedJobs(map[string]string{"backup": "not a cron expression"})}
	err := subject.Reload(reloaded)

	// VERIFY: the reload is refused, naming the job, and the job the
	// scheduler was running is still the one it schedules.
	if err == nil {
		t.Fatalf("Reload() succeeded, want an error for an unusable schedule")
	}
	if !strings.Contains(err.Error(), "backup") {
		t.Errorf("Reload() error = %q, want it to name the job", err.Error())
	}
	if _, ok := subject.Next("backup", time.Now()); !ok {
		t.Errorf("Next(backup) reported no activation, want the running configuration kept")
	}
}

func TestReloadReportsTheSettingsThatNeedARestart(t *testing.T) {
	// SETUP
	backup := helperJob(t, "backup", "ok")
	var reported bytes.Buffer
	subject := buildSchedulerWithLogger(t, loggerInto(t, &reported), backup)

	// EXERCISE: every setting of the scheduler itself changes.
	reloaded := config.Config{
		Scheduler: config.Scheduler{Timezone: "Europe/Rome", MaxParallelJobs: 2},
		Logging: config.Logging{
			Level: "debug", Path: "/var/log/cronx/runs.log", MaxSize: 1024, MaxBackups: 1,
		},
		Storage: config.Storage{Path: "/var/lib/cronx/state.db", MaxRuns: 10},
		Jobs:    reloadedJobs(map[string]string{"backup": "* * * * *"}),
	}
	if err := subject.Reload(reloaded); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY: each of them is named, and the report says why it was ignored.
	log := reported.String()
	for _, setting := range []string{
		"scheduler.timezone",
		"scheduler.max_parallel_jobs",
		"logging.level",
		"logging.path",
		"logging.max_size",
		"logging.max_backups",
		"storage.path",
		"storage.max_runs",
	} {
		if !strings.Contains(log, setting) {
			t.Errorf("the reload reported %q, want %q named as needing a restart", log, setting)
		}
	}
	if !strings.Contains(log, "needs a restart") {
		t.Errorf("the reload reported %q, want it to say that a restart is needed", log)
	}
}

func TestReloadReportsTheJobsItAddedAndRemoved(t *testing.T) {
	// SETUP
	backup := helperJob(t, "backup", "ok")
	var reported bytes.Buffer
	subject := buildSchedulerWithLogger(t, loggerInto(t, &reported), backup)

	// EXERCISE
	reloaded := config.Config{Jobs: reloadedJobs(map[string]string{"cleanup": "* * * * *"})}
	if err := subject.Reload(reloaded); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY
	log := reported.String()
	if !strings.Contains(log, "jobs added by the reload") || !strings.Contains(log, "cleanup") {
		t.Errorf("the reload reported %q, want it to name the job it added", log)
	}
	if !strings.Contains(log, "jobs removed by the reload") || !strings.Contains(log, "backup") {
		t.Errorf("the reload reported %q, want it to name the job it removed", log)
	}
}

func TestReloadPicksUpAJobThatHadNoActivation(t *testing.T) {
	// SETUP: a scheduler with a job that never runs.
	steady := helperJob(t, "steady", "ok")
	steady.Schedule = "0 0 31 4 *"
	subject := buildSchedulerWithLogger(t, loggerInto(t, &bytes.Buffer{}), steady)
	if _, ok := subject.Next("steady", time.Now()); ok {
		t.Fatalf("Next(steady) reported an activation before the reload, want none")
	}

	// EXERCISE: the job becomes one that runs every minute.
	reloaded := config.Config{Jobs: reloadedJobs(map[string]string{"steady": "* * * * *"})}
	if err := subject.Reload(reloaded); err != nil {
		t.Fatalf("Reload() returned an unexpected error: %v", err)
	}

	// VERIFY: the job now has an activation.
	if _, ok := subject.Next("steady", time.Now()); !ok {
		t.Errorf("Next(steady) reported no activation, want the schedule the reload applied")
	}
}
