package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gscacco.com/cronx/internal/cli"
	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

// newTestHome points the commands at an empty home directory and returns it, so
// that the state and the logs of a test never touch the real ones.
func newTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// writeTestConfig writes a configuration file inside the given home.
func writeTestConfig(t *testing.T, home, contents string) string {
	t.Helper()
	directory := filepath.Join(home, ".cronx")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("creating the configuration directory: %v", err)
	}
	path := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing the configuration file: %v", err)
	}
	return path
}

// runCLI executes the command line and returns everything it printed together
// with the error it produced.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := cli.NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

// seedRun records a finished run directly in the database of the given home, so
// that the read-only commands have something to report.
func seedRun(t *testing.T, home, jobName string, status job.Status, startedAt time.Time, exitCode int) {
	t.Helper()
	persistent, err := store.Open(filepath.Join(home, ".cronx", "state.db"))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer func() { _ = persistent.Close() }()

	ctx := context.Background()
	id, err := persistent.StartRun(ctx, jobName, 1, startedAt)
	if err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if err := persistent.FinishRun(ctx, id, store.Finish{
		Status:     status,
		FinishedAt: startedAt.Add(time.Second),
		Duration:   time.Second,
		ExitCode:   &exitCode,
	}); err != nil {
		t.Fatalf("FinishRun() returned an unexpected error: %v", err)
	}
}

// seedLease records, directly in the database of the given home, that a
// scheduler holds the state.
func seedLease(t *testing.T, home, holder string, acquiredAt time.Time, ttl time.Duration) {
	t.Helper()
	persistent, err := store.Open(filepath.Join(home, ".cronx", "state.db"))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	defer func() { _ = persistent.Close() }()

	if _, err := persistent.AcquireLease(context.Background(), holder, acquiredAt, ttl); err != nil {
		t.Fatalf("AcquireLease() returned an unexpected error: %v", err)
	}
}
