package integration_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The rotation tests check that a job which prints a great deal cannot fill the
// disk: the run log is renamed once it passes the configured size, and only the
// number of rotated files the configuration asks for is kept.

// rotatedLogPath returns the path of a rotated run log. The lower the index,
// the newer the file.
func rotatedLogPath(path string, index int) string {
	return fmt.Sprintf("%s.%d", path, index)
}

func TestTheRunLogIsRotatedOnceItPassesItsSize(t *testing.T) {
	// SETUP: a limit that holds one line, and two rotated files to keep.
	environment := newEnvironment(t)
	configPath := environment.configure("rotation.toml")

	// EXERCISE: four runs, so that the first one is dropped.
	for run := 0; run < 4; run++ {
		environment.runOK("run-once", "talker", "--config", configPath)
	}

	// VERIFY: the last three runs, one per file, newest first, and no fourth
	// file.
	last := environment.latestRun("talker")
	files := []struct {
		path string
		id   int64
	}{
		{path: environment.runsLog(), id: last.ID},
		{path: rotatedLogPath(environment.runsLog(), 1), id: last.ID - 1},
		{path: rotatedLogPath(environment.runsLog(), 2), id: last.ID - 2},
	}
	for _, file := range files {
		lines := environment.lines(file.path)
		if len(lines) != 1 {
			t.Fatalf("%s holds %d lines, want one\n%s", file.path, len(lines), strings.Join(lines, "\n"))
		}
		if want := fmt.Sprintf("talker id=%d pid=", file.id); !strings.Contains(lines[0], want) {
			t.Errorf("%s holds %q, want the line of run %d (%s)", file.path, lines[0], file.id, want)
		}
	}
	if _, err := os.Stat(rotatedLogPath(environment.runsLog(), 3)); !os.IsNotExist(err) {
		t.Errorf("a third rotated log was kept, want at most two")
	}
}

func TestTheRunLogIsNotRotatedWhenNoSizeIsConfigured(t *testing.T) {
	// SETUP: the configuration the other tests use says nothing about size.
	environment := newEnvironment(t)
	configPath := environment.configure("logging.toml")

	// EXERCISE
	environment.runOK("run-once", "chatty", "--config", configPath)

	// VERIFY: one log, holding what the run printed.
	if _, err := os.Stat(rotatedLogPath(environment.runsLog(), 1)); !os.IsNotExist(err) {
		t.Errorf("the run log was rotated, want it kept in one file without a configured size")
	}
	if log := environment.readFile(environment.runsLog()); !strings.Contains(log, "chatty stdout") {
		t.Errorf("the run log holds %q, want the output of the run", log)
	}
}
