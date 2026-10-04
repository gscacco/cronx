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

// runLogFiles returns the log files of a job, which are named after the run.
func runLogFiles(t *testing.T, home, jobName string) []string {
	t.Helper()
	directory := filepath.Join(home, ".cronx", "logs", jobName)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("reading the log directory of %q: %v", jobName, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(directory, entry.Name()))
	}
	return paths
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
	logs := runLogFiles(t, home, "hello")
	if len(logs) != 1 {
		t.Fatalf("the run produced %d log files, want 1: %v", len(logs), logs)
	}
	content, err := os.ReadFile(logs[0])
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
