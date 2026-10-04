package integration_test

// The integration tests drive the cronx executable a person installs. Nothing
// in this package links the commands into the test process: every test starts a
// real cronx process, with its own configuration, state database and logs.

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/job"
)

func TestTheBinaryDrivesAConfigurationFileEndToEnd(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("heartbeat.toml")

	// EXERCISE
	validated := environment.runOK("validate", "--config", configPath)
	listed := environment.runOK("list", "--config", configPath)
	executed := environment.runOK("run-once", "heartbeat", "--config", configPath)

	// VERIFY
	if !strings.Contains(validated.stdout, "is valid") {
		t.Errorf("validate printed %q, want it to report the file as valid", validated.stdout)
	}
	for _, want := range []string{"heartbeat", "* * * * *"} {
		if !strings.Contains(listed.stdout, want) {
			t.Errorf("list printed %q, want it to contain %q", listed.stdout, want)
		}
	}
	if !strings.Contains(executed.stdout, `"succeeded"`) {
		t.Errorf("run-once printed %q, want it to report the outcome", executed.stdout)
	}

	// The job ran for real: it left the file it was asked to write behind.
	if content := environment.readFile(environment.path("heartbeat")); content != "ran" {
		t.Errorf("the job wrote %q, want %q", content, "ran")
	}

	// And the run is recorded in the database, where the state lives.
	run := environment.latestRun("heartbeat")
	if run.Status != job.StatusSucceeded {
		t.Errorf("the run was recorded as %q, want %q", run.Status, job.StatusSucceeded)
	}
	if run.ExitCode == nil || *run.ExitCode != 0 {
		t.Errorf("the run recorded the exit code %v, want 0", run.ExitCode)
	}
	if run.LogPath == "" {
		t.Error("the run was recorded without a log file, want the output to be kept")
	}
}
