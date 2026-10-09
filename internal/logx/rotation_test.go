package logx_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/logx"
)

// rotatingRunLog opens a run log that is rotated once it passes maxSize bytes,
// keeping maxBackups of the rotated files, and returns it with its path.
func rotatingRunLog(t *testing.T, maxSize int64, maxBackups int) (*logx.Log, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs", "runs.log")
	return logx.Open(path, clock.Fixed{T: fixedInstant}, logx.Rotation{
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
	}), path
}

// writeRunLine writes one line for a run of a job, as the scheduler would.
func writeRunLine(t *testing.T, subject *logx.Log, job string, id int64, line string) {
	t.Helper()
	writer, err := subject.Writer(job, id)
	if err != nil {
		t.Fatalf("Writer() returned an unexpected error: %v", err)
	}
	writer.SetPID(4242)
	if _, err := writer.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("Write() returned an unexpected error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}
}

// rotatedPath returns the path of the index-th rotated file of a run log. The
// lower the index, the newer the file.
func rotatedPath(path string, index int) string {
	return path + "." + string(rune('0'+index))
}

// exists reports whether a path is there.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// The lines the tests below write are about fifty bytes each, so the sizes they
// configure are about how many lines a log holds rather than about the exact
// width of a timestamp.

func TestTheLogIsRotatedWhenItWouldPassItsSize(t *testing.T) {
	// SETUP: a log that holds two lines at most.
	subject, path := rotatingRunLog(t, 100, 3)

	// EXERCISE
	writeRunLine(t, subject, "backup", 1, "line-1")
	writeRunLine(t, subject, "backup", 2, "line-2")
	writeRunLine(t, subject, "backup", 3, "line-3")

	// VERIFY: the third line started a new file, and the two before it are
	// together in the rotated one.
	current := readFile(t, path)
	if !strings.Contains(current, "line-3") {
		t.Errorf("the run log holds %q, want the line that did not fit", current)
	}
	if strings.Contains(current, "line-1") || strings.Contains(current, "line-2") {
		t.Errorf("the run log holds %q, want only what was written after the rotation", current)
	}
	rotated := readFile(t, rotatedPath(path, 1))
	if !strings.Contains(rotated, "line-1") || !strings.Contains(rotated, "line-2") {
		t.Errorf("the rotated log holds %q, want the lines written before the rotation", rotated)
	}
}

func TestTheOldestRotatedLogIsDroppedWhenTheBackupsAreFull(t *testing.T) {
	// SETUP: two rotated files are kept, so every second line is a rotation.
	subject, path := rotatingRunLog(t, 100, 2)
	lines := []string{"line-1", "line-2", "line-3", "line-4", "line-5", "line-6", "line-7"}

	// EXERCISE
	for index, line := range lines {
		writeRunLine(t, subject, "backup", int64(index+1), line)
	}

	// VERIFY: the two newest rotated files, and no third one.
	first := readFile(t, rotatedPath(path, 1))
	if !strings.Contains(first, "line-5") || !strings.Contains(first, "line-6") {
		t.Errorf("the newest rotated log holds %q, want the fifth and sixth lines", first)
	}
	second := readFile(t, rotatedPath(path, 2))
	if !strings.Contains(second, "line-3") || !strings.Contains(second, "line-4") {
		t.Errorf("the oldest rotated log holds %q, want the third and fourth lines", second)
	}
	if exists(rotatedPath(path, 3)) {
		t.Errorf("a third rotated log was kept, want at most two")
	}
	current := readFile(t, path)
	if !strings.Contains(current, "line-7") {
		t.Errorf("the run log holds %q, want the line written last", current)
	}
}

func TestARotatedLogIsOnlyReadableByItsOwner(t *testing.T) {
	// SETUP
	subject, path := rotatingRunLog(t, 100, 1)
	writeRunLine(t, subject, "backup", 1, "line-1")
	writeRunLine(t, subject, "backup", 2, "line-2")

	// VERIFY
	info, err := os.Stat(rotatedPath(path, 1))
	if err != nil {
		t.Fatalf("inspecting the rotated log: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("rotated log permissions = %v, want %v", got, want)
	}
}

func TestALogWithoutASizeIsNeverRotated(t *testing.T) {
	// SETUP: an installation that says nothing about rotation.
	subject, path := rotatingRunLog(t, 0, 0)

	// EXERCISE
	for index := 0; index < 20; index++ {
		writeRunLine(t, subject, "backup", int64(index+1), "line")
	}

	// VERIFY: every line is in the one file, and no rotated file exists.
	current := readFile(t, path)
	if got, want := strings.Count(current, "\n"), 20; got != want {
		t.Errorf("the run log holds %d lines, want %d", got, want)
	}
	if exists(rotatedPath(path, 1)) {
		t.Errorf("a rotated log was written, want none without a maximum size")
	}
}

func TestTheSizeOfALogLeftByAPreviousProcessIsRespected(t *testing.T) {
	// SETUP: a log written before, without rotation, already past the size a
	// later process is started with.
	path := filepath.Join(t.TempDir(), "logs", "runs.log")
	previous := logx.Open(path, clock.Fixed{T: fixedInstant}, logx.Rotation{})
	for index := 0; index < 3; index++ {
		writeRunLine(t, previous, "backup", int64(index+1), "old-line")
	}
	if err := previous.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}
	subject := logx.Open(path, clock.Fixed{T: fixedInstant}, logx.Rotation{MaxSize: 100, MaxBackups: 1})

	// EXERCISE: the first line of the new process.
	writeRunLine(t, subject, "backup", 4, "new-line")

	// VERIFY: the log that was already too large was rotated rather than
	// grown further.
	current := readFile(t, path)
	if !strings.Contains(current, "new-line") {
		t.Errorf("the run log holds %q, want the line of the new process", current)
	}
	if strings.Contains(current, "old-line") {
		t.Errorf("the run log holds %q, want the lines of the previous process rotated away", current)
	}
	rotated := readFile(t, rotatedPath(path, 1))
	if got, want := strings.Count(rotated, "old-line"), 3; got != want {
		t.Errorf("the rotated log holds %d lines of the previous process, want %d", got, want)
	}
}

func TestALineLargerThanTheLimitIsStillWritten(t *testing.T) {
	// SETUP: a log whose limit is smaller than a single line.
	subject, path := rotatingRunLog(t, 10, 2)

	// EXERCISE
	writeRunLine(t, subject, "backup", 1, "a line that is longer than the limit")

	// VERIFY: the line is in the log rather than rotated away, because there
	// is nothing else the log could hold.
	current := readFile(t, path)
	if !strings.Contains(current, "a line that is longer than the limit") {
		t.Errorf("the run log holds %q, want the line that does not fit either", current)
	}
	if exists(rotatedPath(path, 1)) {
		t.Errorf("the log was rotated for its first line, want the line kept instead")
	}
}
