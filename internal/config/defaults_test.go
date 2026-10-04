package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
)

// minimalConfig sets only the fields that are required.
const minimalConfig = `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
`

func TestParseAppliesDefaults(t *testing.T) {
	// SETUP
	data := []byte(minimalConfig)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Scheduler.Timezone, config.DefaultTimezone; got != want {
		t.Errorf("Scheduler.Timezone = %q, want default %q", got, want)
	}
	if got, want := cfg.Scheduler.MaxParallelJobs, config.DefaultMaxParallelJobs; got != want {
		t.Errorf("Scheduler.MaxParallelJobs = %d, want default %d", got, want)
	}
	if got, want := cfg.Logging.Level, config.DefaultLoggingLevel; got != want {
		t.Errorf("Logging.Level = %q, want default %q", got, want)
	}

	j, ok := cfg.Jobs["backup"]
	if !ok {
		t.Fatalf("job %q not found in %v", "backup", cfg.Jobs)
	}
	if got, want := j.Retry, 0; got != want {
		t.Errorf("Job.Retry = %d, want default %d", got, want)
	}
	if got, want := j.Overlap, job.OverlapSkip; got != want {
		t.Errorf("Job.Overlap = %q, want default %q", got, want)
	}
	if got, want := j.Timeout, time.Duration(0); got != want {
		t.Errorf("Job.Timeout = %v, want default %v (no timeout)", got, want)
	}
	if got, want := len(j.Args), 0; got != want {
		t.Errorf("len(Job.Args) = %d, want default %d", got, want)
	}
	if got, want := j.WorkingDirectory, ""; got != want {
		t.Errorf("Job.WorkingDirectory = %q, want default %q", got, want)
	}
	if got, want := len(j.Env), 0; got != want {
		t.Errorf("len(Job.Env) = %d, want default %d", got, want)
	}
}

func TestLoadReadsConfigurationFile(t *testing.T) {
	// SETUP
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(minimalConfig), 0o600); err != nil {
		t.Fatalf("writing configuration file: %v", err)
	}

	// EXERCISE
	cfg, err := config.Load(path)

	// VERIFY
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}
	if _, ok := cfg.Jobs["backup"]; !ok {
		t.Errorf("job %q not found in %v", "backup", cfg.Jobs)
	}
}

func TestLoadMissingFileReturnsError(t *testing.T) {
	// SETUP
	path := filepath.Join(t.TempDir(), "does-not-exist.toml")

	// EXERCISE
	_, err := config.Load(path)

	// VERIFY
	if err == nil {
		t.Fatalf("Load() succeeded, want an error for a missing file")
	}
}
