package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/cli"
)

// runLogPath returns the path of the log every run writes to: a single file
// shared by all the jobs of the installation.
func runLogPath(home string) string {
	return filepath.Join(home, ".cronx", "logs", "runs.log")
}

func TestRunOnceWritesTheJobOutputToTheRunLog(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, runOnceConfig(t, "print", "0"))

	// EXERCISE
	if _, err := runCLI(t, "run-once", "hello", "--config", path); err != nil {
		t.Fatalf("run-once returned an unexpected error: %v", err)
	}

	// VERIFY
	content, err := os.ReadFile(runLogPath(home))
	if err != nil {
		t.Fatalf("reading the log of the run: %v", err)
	}
	if !strings.Contains(string(content), printedByTheJob) {
		t.Errorf("run log = %q, want it to contain the output of the job", string(content))
	}
}

func TestRunSchedulesUntilItIsStopped(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, `
[scheduler]
timezone = "UTC"

[jobs.later]
schedule = "0 3 * * *"
command = "/usr/local/bin/later"
`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	command := cli.NewRootCommand()
	command.SetContext(ctx)
	command.SetArgs([]string{"run", "--config", path})

	// EXERCISE
	err := command.Execute()

	// VERIFY
	if err != nil {
		t.Fatalf("run returned an unexpected error: %v", err)
	}

	logPath := filepath.Join(home, ".cronx", "logs", "cronx.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading the scheduler log %s: %v", logPath, err)
	}
	if !strings.Contains(string(content), "scheduler started") {
		t.Errorf("scheduler log = %q, want it to record that the scheduler started", string(content))
	}
}
