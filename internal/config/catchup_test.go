package config_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/config"
)

// Catch-up is the option a job asks for to have the activations it missed while
// the scheduler was not running made up for when it starts again. It is off by
// default, and it belongs to the job.

func TestAJobCanAskToBeCaughtUp(t *testing.T) {
	// SETUP
	data := []byte(validJob + "catch_up = true\n")

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if !cfg.Jobs["backup"].CatchUp {
		t.Errorf("the job does not ask to be caught up, want `catch_up = true` to be read")
	}
}

func TestAJobIsNotCaughtUpUnlessItAsks(t *testing.T) {
	// SETUP: a job that says nothing about what it missed.
	data := []byte(validJob)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if cfg.Jobs["backup"].CatchUp {
		t.Errorf("the job asks to be caught up, want catch-up to be off unless it is asked for")
	}
}

func TestCatchUpIsRefusedOutsideAJob(t *testing.T) {
	// SETUP: the key on the scheduler table, where it does not belong.
	data := []byte(validJob + "\n[scheduler]\ncatch_up = true\n")

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
