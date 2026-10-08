package integration_test

import (
	"os"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/job"
)

// invalidConfigurations are configurations that are wrong in exactly one way,
// together with what cronx must say about each of them. They are the mistakes a
// person writes by accident.
var invalidConfigurations = []struct {
	fixture string
	problem string
}{
	{
		fixture: "invalid/schedule.toml",
		problem: `job "backup": schedule is not valid: cron expression "0 3 * *" must have 5 fields, got 4`,
	},
	{
		fixture: "invalid/command.toml",
		problem: `job "backup": command is required`,
	},
	{
		fixture: "invalid/retry.toml",
		problem: `job "backup": retry must not be negative, got -1`,
	},
	{
		fixture: "invalid/timeout.toml",
		problem: `job "backup": timeout "30 minutes" is not a valid duration`,
	},
	{
		fixture: "invalid/overlap.toml",
		problem: `job "backup": overlap "sometimes" is not one of skip, allow, queue`,
	},
	{
		fixture: "invalid/timezone.toml",
		problem: `scheduler.timezone "Europe/Roma" is not a known timezone`,
	},
}

func TestARejectedConfigurationStopsEveryCommand(t *testing.T) {
	for _, invalid := range invalidConfigurations {
		t.Run(invalid.fixture, func(t *testing.T) {
			// SETUP
			environment := newEnvironment(t)
			configPath := environment.configure(invalid.fixture)

			// EXERCISE
			attempts := map[string]result{
				"validate": environment.run("validate", "--config", configPath),
				"run-once": environment.runOnce(configPath, "backup"),
				"run":      environment.run("run", "--config", configPath),
			}

			// VERIFY
			for name, outcome := range attempts {
				if outcome.code == 0 {
					t.Errorf("%s exited with status 0, want a failure for a configuration that is not valid\nstdout:\n%s",
						name, outcome.stdout)
				}
				if !strings.Contains(outcome.stderr, "invalid configuration") {
					t.Errorf("%s printed %q on standard error, want it to report the configuration as invalid",
						name, outcome.stderr)
				}
				if !strings.Contains(outcome.stderr, invalid.problem) {
					t.Errorf("%s printed %q on standard error, want it to explain %q",
						name, outcome.stderr, invalid.problem)
				}
			}

			// Nothing was started, and nothing was left behind.
			if _, err := os.Stat(environment.statePath()); err == nil {
				t.Error("the state database was created, want a rejected configuration to leave no state behind")
			}
			entries, err := os.ReadDir(environment.logsDirectory())
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("reading the log directory: %v", err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					t.Errorf("the log directory %q was created, want no job to have been started", entry.Name())
				}
			}
			if _, err := os.Stat(environment.path("heartbeat")); err == nil {
				t.Error("a job ran, want a rejected configuration not to start anything")
			}
		})
	}
}

func TestARejectedConfigurationLeavesTheRecordedStateAlone(t *testing.T) {
	// SETUP: a job runs and is recorded, so that there is something to protect.
	environment := newEnvironment(t)
	valid := environment.configure("heartbeat.toml")
	environment.runOK("run-once", "heartbeat", "--config", valid)
	recorded := environment.latestRun("heartbeat")
	stateBefore := environment.stateInfo()

	// EXERCISE: every rejected configuration is fed to every command.
	for _, invalid := range invalidConfigurations {
		configPath := environment.configure(invalid.fixture)
		environment.run("validate", "--config", configPath)
		environment.runOnce(configPath, "backup")
		environment.run("run", "--config", configPath)
	}

	// VERIFY
	if stateAfter := environment.stateInfo(); !os.SameFile(stateBefore, stateAfter) {
		t.Error("the state database was replaced, want a rejected configuration to leave it untouched")
	}
	kept := environment.latestRun("heartbeat")
	if kept.ID != recorded.ID || kept.Status != recorded.Status || kept.LogPath != recorded.LogPath {
		t.Errorf("the recorded run is now run %d, %q, log %s, want run %d, %q, log %s",
			kept.ID, kept.Status, kept.LogPath, recorded.ID, recorded.Status, recorded.LogPath)
	}
	if runs := environment.runs("heartbeat"); len(runs) != 1 {
		t.Errorf("%d runs are recorded, want only the one that ran", len(runs))
	}
	history := environment.runOK("history", "heartbeat", "--config", valid)
	if !strings.Contains(history.stdout, string(job.StatusSucceeded)) {
		t.Errorf("history printed %q, want the run that was recorded before", history.stdout)
	}
}

func TestTheConfigurationIsFoundTheDocumentedWay(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	fromHome := environment.configureAt("heartbeat.toml", environment.defaultConfigPath())
	fromVariable := environment.configure("heartbeat.toml")
	environmentRun := []string{"CRONX_CONFIG=" + fromVariable}

	// EXERCISE: the flag wins over the environment variable, which wins over
	// the default location in the home directory.
	byDefault := environment.runOK("validate")
	byVariable := environment.runWithEnvironment(environmentRun, "validate")
	byFlag := environment.runWithEnvironment(environmentRun, "validate", "--config", fromHome)

	// VERIFY
	for _, expected := range []struct {
		description string
		outcome     result
		path        string
	}{
		{description: "the default location in the home directory", outcome: byDefault, path: fromHome},
		{description: "CRONX_CONFIG", outcome: byVariable, path: fromVariable},
		{description: "the --config flag", outcome: byFlag, path: fromHome},
	} {
		if !strings.Contains(expected.outcome.stdout, expected.path) {
			t.Errorf("validate printed %q when the configuration came from %s, want it to report %s",
				expected.outcome.stdout, expected.description, expected.path)
		}
	}
}
