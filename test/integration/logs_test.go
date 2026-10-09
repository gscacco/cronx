package integration_test

import (
	"strings"
	"testing"
)

// The logs tests check that the run log can be read through the command line,
// which is how a person reaches the output of a run without knowing where the
// log lives.

func TestTheLogsCommandPrintsWhatTheRunsPrinted(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("logging.toml")

	// EXERCISE: one job runs, the other does not.
	environment.runOK("run-once", "chatty", "--config", configPath)
	chatty := environment.runOK("logs", "chatty", "--config", configPath)
	noisy := environment.runOK("logs", "noisy", "--config", configPath)
	everyJob := environment.runOK("logs", "--config", configPath)

	// VERIFY: both streams of the run that happened, and nothing of the job
	// that did not run.
	for _, text := range []string{"chatty stdout", "chatty stderr"} {
		if !strings.Contains(chatty.stdout, text) {
			t.Errorf("cronx logs chatty printed %q, want it to hold %q", chatty.stdout, text)
		}
	}
	if noisy.stdout != "" {
		t.Errorf("cronx logs noisy printed %q, want nothing: that job never ran", noisy.stdout)
	}
	if !strings.Contains(everyJob.stdout, "chatty stdout") {
		t.Errorf("cronx logs printed %q, want the lines of every job", everyJob.stdout)
	}

	// Every line is printed as the run log holds it, so that it can be
	// compared with the file and with what history reports.
	for _, line := range strings.Split(strings.TrimRight(chatty.stdout, "\n"), "\n") {
		if !strings.Contains(line, "chatty id=") {
			t.Errorf("cronx logs printed the line %q, want the run log line as it is", line)
		}
		if !timestampPattern.MatchString(line) {
			t.Errorf("cronx logs printed the line %q without a leading timestamp", line)
		}
	}
}

func TestTheLogsCommandSaysNothingBeforeAnythingRan(t *testing.T) {
	// SETUP: nothing has run, so no run log was ever created.
	environment := newEnvironment(t)
	configPath := environment.configure("logging.toml")

	// EXERCISE
	outcome := environment.runOK("logs", "--config", configPath)

	// VERIFY: a command meant to be piped prints nothing, and succeeds.
	if outcome.stdout != "" {
		t.Errorf("cronx logs printed %q, want nothing before the first run", outcome.stdout)
	}
}
