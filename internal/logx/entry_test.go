package logx_test

import (
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/logx"
)

func TestALogLineIsSplitIntoItsFields(t *testing.T) {
	// SETUP: a line exactly as the run writer stamps it.
	line := "2026-01-02T03:04:05Z backup id=7 pid=4242 starting the backup"

	// EXERCISE
	entry, err := logx.ParseLine(line)

	// VERIFY
	if err != nil {
		t.Fatalf("ParseLine() returned an unexpected error: %v", err)
	}
	if !entry.Time.Equal(fixedInstant) {
		t.Errorf("Entry.Time = %s, want %s", entry.Time, fixedInstant)
	}
	if entry.Job != "backup" {
		t.Errorf("Entry.Job = %q, want %q", entry.Job, "backup")
	}
	if entry.RunID != 7 {
		t.Errorf("Entry.RunID = %d, want 7", entry.RunID)
	}
	if entry.PID != 4242 {
		t.Errorf("Entry.PID = %d, want 4242", entry.PID)
	}
	if want := "starting the backup"; entry.Message != want {
		t.Errorf("Entry.Message = %q, want %q", entry.Message, want)
	}
}

func TestALogLineWithoutAMessageIsStillALineOfTheRunLog(t *testing.T) {
	// SETUP: a job that prints an empty line produces a line with no message,
	// which the writer still terminates with a space.
	line := "2026-01-02T03:04:05Z backup id=7 pid=4242 "

	// EXERCISE
	entry, err := logx.ParseLine(line)

	// VERIFY
	if err != nil {
		t.Fatalf("ParseLine() returned an unexpected error: %v", err)
	}
	if entry.Message != "" {
		t.Errorf("Entry.Message = %q, want it to be empty", entry.Message)
	}
}

func TestALogLineKeepsWhatTheJobPrinted(t *testing.T) {
	// SETUP: anything a job prints can reach the log, spaces and fields that
	// look like the prefix of a run included.
	line := "2026-01-02T03:04:05Z backup id=7 pid=4242 id=9 pid=9 two  spaces"

	// EXERCISE
	entry, err := logx.ParseLine(line)

	// VERIFY
	if err != nil {
		t.Fatalf("ParseLine() returned an unexpected error: %v", err)
	}
	if want := "id=9 pid=9 two  spaces"; entry.Message != want {
		t.Errorf("Entry.Message = %q, want %q", entry.Message, want)
	}
}

func TestALogLineCarriesTheInstantInAnyZone(t *testing.T) {
	// SETUP: the writer stamps lines with the zone of the machine, so a line
	// is not always written with a trailing Z.
	line := "2026-01-02T03:04:05+01:00 backup id=7 pid=4242 hello"

	// EXERCISE
	entry, err := logx.ParseLine(line)

	// VERIFY
	if err != nil {
		t.Fatalf("ParseLine() returned an unexpected error: %v", err)
	}
	if want := fixedInstant.Add(-time.Hour); !entry.Time.Equal(want) {
		t.Errorf("Entry.Time = %s, want the same instant as %s", entry.Time, want)
	}
}

func TestALineThatIsNotARunLogLineIsRejected(t *testing.T) {
	// SETUP: the lines a file that is not the run log holds, and the lines a
	// run log cannot hold.
	for _, line := range []string{
		"",
		"this is not a run log line",
		"2026-01-02T03:04:05Z backup",
		"not-an-instant backup id=7 pid=4242 hello",
		"2026-01-02T03:04:05Z backup run=7 pid=4242 hello",
		"2026-01-02T03:04:05Z backup id=seven pid=4242 hello",
		"2026-01-02T03:04:05Z  id=7 pid=4242 hello",
	} {
		// EXERCISE
		_, err := logx.ParseLine(line)

		// VERIFY
		if err == nil {
			t.Errorf("ParseLine(%q) succeeded, want an error", line)
		}
	}
}

func TestWhatTheWriterStampsIsWhatTheParserReads(t *testing.T) {
	// SETUP: a run whose output is written by the writer, to be read back by
	// the parser. The two must agree on the shape of a line.
	subject, path := newRunLog(t)
	writer, err := subject.Writer("backup", 7)
	if err != nil {
		t.Fatalf("Writer() returned an unexpected error: %v", err)
	}
	writer.SetPID(4242)
	if _, err := writer.Write([]byte("hello from the job\n")); err != nil {
		t.Fatalf("Write() returned an unexpected error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}

	// EXERCISE
	entry, err := logx.ParseLine(strings.TrimRight(readFile(t, path), "\n"))

	// VERIFY
	if err != nil {
		t.Fatalf("ParseLine() returned an unexpected error: %v", err)
	}
	if entry.Job != "backup" || entry.RunID != 7 || entry.PID != 4242 {
		t.Errorf("Entry = %+v, want the job, the run and the pid the writer stamped", entry)
	}
	if want := "hello from the job"; entry.Message != want {
		t.Errorf("Entry.Message = %q, want %q", entry.Message, want)
	}
}
