package integration_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The logging tests check that the output of a run is kept where the
// documentation says it is, that neither stream is lost, and that nothing but
// its owner can read it.

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

	for _, expected := range []struct {
		name   string
		stdout string
		stderr string
	}{
		{name: "chatty", stdout: "chatty stdout", stderr: "chatty stderr"},
		{name: "noisy", stdout: "noisy stdout", stderr: "noisy stderr"},
	} {
		run := environment.latestRun(expected.name)

		// The log is where the run says it is, and where the documentation
		// says the log of a run lives: <home>/.cronx/logs/<job>/<run id>.log.
		wanted := filepath.Join(environment.jobLogDirectory(expected.name), fmt.Sprintf("%d.log", run.ID))
		if run.LogPath != wanted {
			t.Errorf("run %d of %q logged to %q, want %q", run.ID, expected.name, run.LogPath, wanted)
		}
		output := environment.readFile(wanted)
		for _, text := range []string{expected.stdout, expected.stderr} {
			if !strings.Contains(output, text) {
				t.Errorf("the log of run %d of %q holds %q, want it to contain %q",
					run.ID, expected.name, output, text)
			}
		}

		// A log can hold anything a job prints, so only its owner may read it.
		if mode := permissionMode(t, wanted); mode != 0o600 {
			t.Errorf("the log of run %d is readable as %o, want 600", run.ID, mode)
		}
		if mode := permissionMode(t, environment.jobLogDirectory(expected.name)); mode != 0o700 {
			t.Errorf("the log directory of %q is readable as %o, want 700", expected.name, mode)
		}
	}

	// How a person finds the log of a run: the history reports the identifier
	// the file is named after.
	history := environment.runOK("history", "--config", configPath)
	for _, name := range []string{"chatty", "noisy"} {
		run := environment.latestRun(name)
		rows := historyRows(t, history.stdout, name)
		if len(rows) == 0 || rows[0].id != strconv.FormatInt(run.ID, 10) {
			t.Errorf("history does not report run %d of %q as its newest run\n%s", run.ID, name, history.stdout)
		}
	}
}
