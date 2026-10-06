package integration_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The logging tests check that the output of a run is kept where the
// documentation says it is, that neither stream is lost, that every line
// identifies the run that produced it, and that nothing but its owner can read
// it.

// timestampPattern matches the readable timestamp every line of the run log
// begins with.
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)

func TestTheOutputOfBothStreamsIsKeptInTheRunLog(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("logging.toml")

	// EXERCISE
	environment.runOK("run-once", "chatty", "--config", configPath)
	failed := environment.runOnce(configPath, "noisy")

	// VERIFY
	if failed.code == 0 {
		t.Fatalf("run-once succeeded, want a failure for a job that exits 4\nstdout:\n%s", failed.stdout)
	}

	output := environment.readFile(environment.runsLog())
	for _, expected := range []struct {
		name   string
		stdout string
		stderr string
	}{
		{name: "chatty", stdout: "chatty stdout", stderr: "chatty stderr"},
		{name: "noisy", stdout: "noisy stdout", stderr: "noisy stderr"},
	} {
		run := environment.latestRun(expected.name)

		// Every run appends to the one log the database points at.
		if run.LogPath != environment.runsLog() {
			t.Errorf("run %d of %q logged to %q, want the shared run log %q",
				run.ID, expected.name, run.LogPath, environment.runsLog())
		}

		// Neither stream is lost.
		for _, text := range []string{expected.stdout, expected.stderr} {
			if !strings.Contains(output, text) {
				t.Errorf("the run log holds %q, want it to contain %q", output, text)
			}
		}

		// Every line of a run identifies the process that produced it.
		prefix := expected.name + " id=" + strconv.FormatInt(run.ID, 10) + " pid="
		if !strings.Contains(output, prefix) {
			t.Errorf("the run log holds %q, want it to identify run %d of %q with %q",
				output, run.ID, expected.name, prefix)
		}
	}

	// Every line carries a readable timestamp.
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if !timestampPattern.MatchString(line) {
			t.Errorf("the run log holds the line %q without a leading timestamp", line)
		}
	}

	// A log can hold anything a job prints, so only its owner may read it.
	if mode := permissionMode(t, environment.runsLog()); mode != 0o600 {
		t.Errorf("the run log is readable as %o, want 600", mode)
	}
	if mode := permissionMode(t, environment.logsDirectory()); mode != 0o700 {
		t.Errorf("the log directory is readable as %o, want 700", mode)
	}

	// How a person finds the log of a run: the history reports the identifier
	// the run was recorded with, which is the one the log carries on its lines.
	history := environment.runOK("history", "--config", configPath)
	for _, name := range []string{"chatty", "noisy"} {
		run := environment.latestRun(name)
		rows := historyRows(t, history.stdout, name)
		if len(rows) == 0 || rows[0].id != strconv.FormatInt(run.ID, 10) {
			t.Errorf("history does not report run %d of %q as its newest run\n%s",
				run.ID, name, history.stdout)
		}
	}
}
