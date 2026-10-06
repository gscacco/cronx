package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The configuration can move the run log and the status database out of the
// home directory. This test checks that both paths are honoured, and that
// nothing is left where the defaults would have put it.

func TestTheLogAndTheStateDatabaseLiveWhereTheConfigurationSays(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("paths.toml")
	configuredLog := environment.path("custom/runs.log")
	configuredState := environment.path("custom/state.db")

	// EXERCISE
	environment.runOK("run-once", "chatty", "--config", configPath)

	// VERIFY
	output := environment.readFile(configuredLog)
	if !strings.Contains(output, "chatty stdout") {
		t.Errorf("the configured log holds %q, want the output of the job", output)
	}
	if !strings.Contains(output, "chatty id=") {
		t.Errorf("the configured log holds %q, want it to identify the run", output)
	}
	if _, err := os.Stat(configuredState); err != nil {
		t.Errorf("the configured state database does not exist: %v", err)
	}

	// Nothing is written where the defaults would have gone.
	if _, err := os.Stat(environment.statePath()); err == nil {
		t.Errorf("the default state database %s was created, want the configured one only", environment.statePath())
	}
	if _, err := os.Stat(filepath.Join(environment.logsDirectory(), "runs.log")); err == nil {
		t.Error("the default run log was created, want the configured one only")
	}
}
