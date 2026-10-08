package config_test

import (
	"testing"
	"time"

	"gscacco.com/cronx/internal/config"
)

// The grace period is how long a job is given to stop after SIGTERM before it
// is killed. A job that does not choose one carries the zero value, which is
// how the runner is told to apply its own default.

func TestAJobCarriesTheConfiguredGracePeriod(t *testing.T) {
	// SETUP
	data := []byte(`
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
grace_period = "2s"
`)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Jobs["backup"].GracePeriod, 2*time.Second; got != want {
		t.Errorf("the grace period of the job is %s, want %s", got, want)
	}
}

func TestAJobWithoutAGracePeriodLeavesTheDefaultToTheRunner(t *testing.T) {
	// SETUP
	data := []byte(minimalConfig)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got := cfg.Jobs["backup"].GracePeriod; got != 0 {
		t.Errorf("the grace period of a job that does not set one is %s, want the zero value", got)
	}
}
