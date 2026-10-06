package logx_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/logx"
)

// fixedInstant is the instant the tests of the run log stamp their lines with.
// It is rendered by the run log as an RFC 3339 timestamp.
var fixedInstant = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// stampedInstant is how fixedInstant appears in the log.
const stampedInstant = "2026-01-02T03:04:05Z"

// newRunLog opens a run log inside a fresh temporary directory and returns it
// together with the path it writes to.
func newRunLog(t *testing.T) (*logx.Log, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs", "runs.log")
	return logx.Open(path, clock.Fixed{T: fixedInstant}), path
}

// readFile returns the contents of a file, failing the test when it cannot be
// read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(content)
}

func TestNothingIsWrittenBeforeTheFirstRun(t *testing.T) {
	// SETUP
	subject, path := newRunLog(t)

	// EXERCISE
	err := subject.Close()

	// VERIFY
	if err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the log file exists, want nothing written before the first run")
	}
}

func TestWriterCreatesTheFileItsDirectoryDoesNotExistYet(t *testing.T) {
	// SETUP
	subject, path := newRunLog(t)

	// EXERCISE
	writer, err := subject.Writer("backup", 7)

	// VERIFY
	if err != nil {
		t.Fatalf("Writer() returned an unexpected error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the run log was not created: %v", err)
	}
}

func TestEveryLineCarriesTheInstantTheJobTheRunAndThePid(t *testing.T) {
	// SETUP
	subject, path := newRunLog(t)
	writer, err := subject.Writer("backup", 7)
	if err != nil {
		t.Fatalf("Writer() returned an unexpected error: %v", err)
	}
	writer.SetPID(4242)

	// EXERCISE
	if _, err := writer.Write([]byte("first line\nsecond line\n")); err != nil {
		t.Fatalf("Write() returned an unexpected error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}

	// VERIFY
	want := stampedInstant + " backup id=7 pid=4242 first line\n" +
		stampedInstant + " backup id=7 pid=4242 second line\n"
	if got := readFile(t, path); got != want {
		t.Errorf("run log = %q, want %q", got, want)
	}
}

func TestLinesAreHeldUntilThePidIsKnown(t *testing.T) {
	// SETUP
	subject, path := newRunLog(t)
	writer, err := subject.Writer("backup", 7)
	if err != nil {
		t.Fatalf("Writer() returned an unexpected error: %v", err)
	}

	// EXERCISE: the process prints before the runner has reported its pid.
	if _, err := writer.Write([]byte("early\n")); err != nil {
		t.Fatalf("Write() returned an unexpected error: %v", err)
	}
	if got := readFile(t, path); got != "" {
		t.Errorf("run log = %q, want nothing written before the pid is known", got)
	}
	writer.SetPID(99)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}

	// VERIFY
	wanted := stampedInstant + " backup id=7 pid=99 early\n"
	if got := readFile(t, path); got != wanted {
		t.Errorf("run log = %q, want %q", got, wanted)
	}
}

func TestATrailingLineWithoutANewlineIsStillWritten(t *testing.T) {
	// SETUP
	subject, path := newRunLog(t)
	writer, err := subject.Writer("backup", 7)
	if err != nil {
		t.Fatalf("Writer() returned an unexpected error: %v", err)
	}
	writer.SetPID(1)

	// EXERCISE
	if _, err := writer.Write([]byte("no trailing newline")); err != nil {
		t.Fatalf("Write() returned an unexpected error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}

	// VERIFY
	wanted := stampedInstant + " backup id=7 pid=1 no trailing newline\n"
	if got := readFile(t, path); got != wanted {
		t.Errorf("run log = %q, want %q", got, wanted)
	}
}

func TestEveryRunSharesTheSameFile(t *testing.T) {
	// SETUP
	subject, path := newRunLog(t)

	// EXERCISE
	for _, run := range []struct {
		job string
		id  int64
		pid int
	}{{"backup", 1, 10}, {"metrics", 2, 20}} {
		writer, err := subject.Writer(run.job, run.id)
		if err != nil {
			t.Fatalf("Writer(%q) returned an unexpected error: %v", run.job, err)
		}
		writer.SetPID(run.pid)
		if _, err := writer.Write([]byte(run.job + " ran\n")); err != nil {
			t.Fatalf("Write() returned an unexpected error: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Close() returned an unexpected error: %v", err)
		}
	}

	// VERIFY
	got := readFile(t, path)
	for _, want := range []string{
		"backup id=1 pid=10 backup ran\n",
		"metrics id=2 pid=20 metrics ran\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("run log = %q, want it to contain %q", got, want)
		}
	}
}

func TestTheRunLogAndItsDirectoryAreOnlyReadableByTheirOwner(t *testing.T) {
	// SETUP
	subject, path := newRunLog(t)
	writer, err := subject.Writer("backup", 1)
	if err != nil {
		t.Fatalf("Writer() returned an unexpected error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}

	// VERIFY
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("inspecting the run log: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("run log permissions = %v, want %v", got, want)
	}
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("inspecting the log directory: %v", err)
	}
	if got, want := directory.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Errorf("log directory permissions = %v, want %v", got, want)
	}
}

func TestConcurrentRunsDoNotInterleaveTheirLines(t *testing.T) {
	// SETUP
	subject, path := newRunLog(t)
	const runs = 4
	const perRun = 100
	linePattern := regexp.MustCompile(`^` + regexp.QuoteMeta(stampedInstant) + ` job\d+ id=\d+ pid=\d+ line-\d+$`)

	// EXERCISE
	var wait sync.WaitGroup
	for index := 0; index < runs; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			writer, err := subject.Writer(fmt.Sprintf("job%d", index), int64(index))
			if err != nil {
				t.Errorf("Writer() returned an unexpected error: %v", err)
				return
			}
			writer.SetPID(100 + index)
			for line := 0; line < perRun; line++ {
				if _, err := writer.Write([]byte(fmt.Sprintf("line-%d\n", line))); err != nil {
					t.Errorf("Write() returned an unexpected error: %v", err)
					return
				}
			}
			_ = writer.Close()
		}(index)
	}
	wait.Wait()

	// VERIFY: every line is whole, and no line was lost to a concurrent run.
	lines := strings.Split(strings.TrimRight(readFile(t, path), "\n"), "\n")
	if len(lines) != runs*perRun {
		t.Fatalf("the log holds %d lines, want %d", len(lines), runs*perRun)
	}
	for _, line := range lines {
		if !linePattern.MatchString(line) {
			t.Errorf("the log holds the malformed line %q", line)
		}
	}
}
