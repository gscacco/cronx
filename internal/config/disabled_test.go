package config_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/config"
)

// A job is scheduled unless the configuration turns it off, which is what
// `enabled = false` does: the definition is kept — `cronx list` names it and its
// history stays where it is — and it is not run.

func TestAJobIsScheduledUnlessTheConfigurationDisablesIt(t *testing.T) {
	// SETUP: a job that says nothing about being enabled.
	data := []byte(validJob)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if cfg.Jobs["backup"].Disabled {
		t.Errorf("the job is disabled, want a job that says nothing about it to be scheduled")
	}
}

func TestAJobCanBeKeptWithoutBeingScheduled(t *testing.T) {
	// SETUP
	data := []byte(validJob + "enabled = false\n")

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if !cfg.Jobs["backup"].Disabled {
		t.Errorf("the job is scheduled, want `enabled = false` to keep it without running it")
	}
}

func TestEnablingAJobExplicitlyKeepsItScheduled(t *testing.T) {
	// SETUP
	data := []byte(validJob + "enabled = true\n")

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if cfg.Jobs["backup"].Disabled {
		t.Errorf("the job is disabled, want `enabled = true` to leave it scheduled")
	}
}

func TestEnabledIsRefusedOutsideAJob(t *testing.T) {
	// SETUP: the key on the scheduler table, where it does not belong.
	data := []byte(validJob + "\n[scheduler]\nenabled = false\n")

	// EXERCISE
	_, err := config.Parse(data)

	// VERIFY
	if err == nil {
		t.Fatalf("Parse() succeeded, want the key to be refused outside a job")
	}
	if !strings.Contains(err.Error(), "unknown configuration key") {
		t.Errorf("Parse() error = %q, want it to report an unknown key", err.Error())
	}
}
