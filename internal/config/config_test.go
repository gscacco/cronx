package config_test

import (
	"testing"
	"time"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
)

// fullConfig sets every supported field to an explicit value.
const fullConfig = `
[scheduler]
timezone = "Europe/Rome"
max_parallel_jobs = 2

[logging]
level = "debug"

[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
args = ["--incremental", "--destination", "/backup"]
timeout = "30m"
retry = 2
overlap = "skip"
working_directory = "/var/lib/backup"
env = { TIER = "gold" }
`

const (
	wantFullMaxParallelJobs = 2
	wantFullRetry           = 2
	wantFullTimeout         = 30 * time.Minute
	wantFullArgs            = 3
)

func TestParseFullConfig(t *testing.T) {
	// SETUP
	data := []byte(fullConfig)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Scheduler.Timezone, "Europe/Rome"; got != want {
		t.Errorf("Scheduler.Timezone = %q, want %q", got, want)
	}
	if got, want := cfg.Scheduler.MaxParallelJobs, wantFullMaxParallelJobs; got != want {
		t.Errorf("Scheduler.MaxParallelJobs = %d, want %d", got, want)
	}
	if got, want := cfg.Logging.Level, "debug"; got != want {
		t.Errorf("Logging.Level = %q, want %q", got, want)
	}

	j, ok := cfg.Jobs["backup"]
	if !ok {
		t.Fatalf("job %q not found in %v", "backup", cfg.Jobs)
	}
	if got, want := j.Name, "backup"; got != want {
		t.Errorf("Job.Name = %q, want %q", got, want)
	}
	if got, want := j.Schedule, "0 3 * * *"; got != want {
		t.Errorf("Job.Schedule = %q, want %q", got, want)
	}
	if got, want := j.Command, "/usr/local/bin/backup"; got != want {
		t.Errorf("Job.Command = %q, want %q", got, want)
	}
	if got, want := len(j.Args), wantFullArgs; got != want {
		t.Fatalf("len(Job.Args) = %d, want %d", got, want)
	}
	if got, want := j.Args[0], "--incremental"; got != want {
		t.Errorf("Job.Args[0] = %q, want %q", got, want)
	}
	if got, want := j.Args[1], "--destination"; got != want {
		t.Errorf("Job.Args[1] = %q, want %q", got, want)
	}
	if got, want := j.Args[2], "/backup"; got != want {
		t.Errorf("Job.Args[2] = %q, want %q", got, want)
	}
	if got, want := j.Timeout, wantFullTimeout; got != want {
		t.Errorf("Job.Timeout = %v, want %v", got, want)
	}
	if got, want := j.Retry, wantFullRetry; got != want {
		t.Errorf("Job.Retry = %d, want %d", got, want)
	}
	if got, want := j.Overlap, job.OverlapSkip; got != want {
		t.Errorf("Job.Overlap = %q, want %q", got, want)
	}
	if got, want := j.WorkingDirectory, "/var/lib/backup"; got != want {
		t.Errorf("Job.WorkingDirectory = %q, want %q", got, want)
	}
	if got, want := j.Env["TIER"], "gold"; got != want {
		t.Errorf("Job.Env[TIER] = %q, want %q", got, want)
	}
}
