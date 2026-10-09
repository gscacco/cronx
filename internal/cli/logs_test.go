package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gscacco.com/cronx/internal/cli"
)

// writeRunLog writes the given lines to a run log, terminated by a newline, as
// the run writer would.
func writeRunLog(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("creating the log directory: %v", err)
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the run log: %v", err)
	}
}

// appendToRunLog appends a line to a run log.
func appendToRunLog(t *testing.T, path, line string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("opening the run log: %v", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(line + "\n"); err != nil {
		t.Fatalf("appending to the run log: %v", err)
	}
}

// synchronizedBuffer is a buffer two goroutines can share: the command writes
// to it while the test reads what it has written so far.
type synchronizedBuffer struct {
	mutex sync.Mutex
	text  bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.text.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.text.String()
}

// waitForOutput waits until the buffer holds the text, failing the test when it
// does not appear within a few seconds.
func waitForOutput(t *testing.T, output *synchronizedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the output never held %q\noutput:\n%s", want, output.String())
}

func TestLogsPrintsTheLinesOfAJob(t *testing.T) {
	// SETUP: two jobs wrote to the one run log.
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	writeRunLog(t, runLogPath(home),
		"2026-01-02T03:00:00Z backup id=1 pid=10 starting",
		"2026-01-02T04:00:00Z cleanup id=2 pid=20 starting",
		"2026-01-02T03:00:01Z backup id=1 pid=10 done",
	)

	// EXERCISE
	output, err := runCLI(t, "logs", "backup", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("logs returned an unexpected error: %v", err)
	}
	want := "2026-01-02T03:00:00Z backup id=1 pid=10 starting\n" +
		"2026-01-02T03:00:01Z backup id=1 pid=10 done\n"
	if output != want {
		t.Errorf("logs output = %q, want %q", output, want)
	}

	// Reading the log is not a reason to create the state.
	if _, err := os.Stat(filepath.Join(home, ".cronx", "state.db")); !os.IsNotExist(err) {
		t.Errorf("logs created the state database, want a command that only reads the log")
	}
}

func TestLogsPrintsEveryJobWhenNoJobIsNamed(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	writeRunLog(t, runLogPath(home),
		"2026-01-02T03:00:00Z backup id=1 pid=10 starting",
		"2026-01-02T04:00:00Z cleanup id=2 pid=20 starting",
	)

	// EXERCISE
	output, err := runCLI(t, "logs", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("logs returned an unexpected error: %v", err)
	}
	for _, want := range []string{"backup id=1", "cleanup id=2"} {
		if !strings.Contains(output, want) {
			t.Errorf("logs output = %q, want it to contain %q", output, want)
		}
	}
}

func TestLogsLeavesOutTheLinesOlderThanSince(t *testing.T) {
	// SETUP: the same job wrote two days ago and a minute ago.
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	old := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	recent := time.Now().Add(-time.Minute).Format(time.RFC3339)
	writeRunLog(t, runLogPath(home),
		old+" backup id=1 pid=10 long ago",
		recent+" backup id=2 pid=11 just now",
	)

	// EXERCISE
	output, err := runCLI(t, "logs", "backup", "--since", "1h", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("logs returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "just now") {
		t.Errorf("logs output = %q, want the line written within the last hour", output)
	}
	if strings.Contains(output, "long ago") {
		t.Errorf("logs output = %q, want the line written two days ago to be left out", output)
	}
}

func TestLogsAcceptsAnInstantAsSince(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	writeRunLog(t, runLogPath(home),
		"2026-01-02T03:00:00Z backup id=1 pid=10 old",
		"2026-01-02T05:00:00Z backup id=2 pid=11 new",
	)

	// EXERCISE
	output, err := runCLI(t, "logs", "backup", "--since", "2026-01-02T04:00:00Z", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("logs returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "new") || strings.Contains(output, "old") {
		t.Errorf("logs output = %q, want only the line written after the instant", output)
	}
}

func TestLogsRejectsASinceItCannotRead(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)

	for _, value := range []string{"yesterday", "-5m"} {
		// EXERCISE
		_, err := runCLI(t, "logs", "--since", value, "--config", path)

		// VERIFY
		if err == nil {
			t.Errorf("logs --since %q succeeded, want an error", value)
		}
	}
}

func TestLogsSaysNothingWhenTheRunLogDoesNotExist(t *testing.T) {
	// SETUP: nothing has run yet, so no line was ever written.
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)

	// EXERCISE
	output, err := runCLI(t, "logs", "--config", path)

	// VERIFY: a command meant to be piped writes nothing rather than a message.
	if err != nil {
		t.Fatalf("logs returned an unexpected error: %v", err)
	}
	if output != "" {
		t.Errorf("logs output = %q, want nothing", output)
	}
}

func TestLogsLeavesOutWhatIsNotARunLogLine(t *testing.T) {
	// SETUP: a file that is not the run log, or one a person appended to.
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	writeRunLog(t, runLogPath(home),
		"this is not a run log line",
		"2026-01-02T03:00:00Z backup id=1 pid=10 a real one",
	)

	// EXERCISE
	output, err := runCLI(t, "logs", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("logs returned an unexpected error: %v", err)
	}
	if want := "2026-01-02T03:00:00Z backup id=1 pid=10 a real one\n"; output != want {
		t.Errorf("logs output = %q, want %q", output, want)
	}
}

func TestLogsReadsTheLogTheConfigurationMoves(t *testing.T) {
	// SETUP: the run log lives somewhere else than under the home directory.
	home := newTestHome(t)
	moved := filepath.Join(home, "elsewhere", "runs.log")
	path := writeTestConfig(t, home, fmt.Sprintf(`
[logging]
path = %q

[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
`, moved))
	writeRunLog(t, moved, "2026-01-02T03:00:00Z backup id=1 pid=10 moved")

	// EXERCISE
	output, err := runCLI(t, "logs", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("logs returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "moved") {
		t.Errorf("logs output = %q, want the line of the log the configuration points at", output)
	}
}

func TestLogsFollowsTheLog(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	logPath := runLogPath(home)
	writeRunLog(t, logPath, "2026-01-02T03:00:00Z backup id=1 pid=10 first")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := &synchronizedBuffer{}
	command := cli.NewRootCommand()
	command.SetOut(output)
	command.SetErr(output)
	command.SetContext(ctx)
	command.SetArgs([]string{"logs", "--follow", "--config", path})

	finished := make(chan error, 1)
	go func() { finished <- command.Execute() }()

	// EXERCISE: what was already written, then what arrives later.
	waitForOutput(t, output, "first")
	appendToRunLog(t, logPath, "2026-01-02T03:01:00Z cleanup id=2 pid=20 second")

	// VERIFY
	waitForOutput(t, output, "second")
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("logs returned an unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("logs did not stop when the command was interrupted\noutput:\n%s", output.String())
	}
}
