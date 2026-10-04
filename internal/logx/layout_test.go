package logx_test

import (
	"os"
	"path/filepath"
	"testing"

	"gscacco.com/cronx/internal/logx"
)

func TestDefaultLayoutLivesUnderTheHomeDirectory(t *testing.T) {
	// SETUP
	home := t.TempDir()
	t.Setenv("HOME", home)

	// EXERCISE
	layout, err := logx.DefaultLayout()

	// VERIFY
	if err != nil {
		t.Fatalf("DefaultLayout() returned an unexpected error: %v", err)
	}
	want := filepath.Join(home, ".cronx", "logs")
	if layout.Root() != want {
		t.Errorf("Layout.Root() = %q, want %q", layout.Root(), want)
	}
}

func TestPathsFollowTheDocumentedLayout(t *testing.T) {
	// SETUP
	layout := logx.NewLayout("/var/log/cronx")

	// EXERCISE
	runPath := layout.RunPath("backup", 42)
	schedulerPath := layout.SchedulerPath()

	// VERIFY
	if want := filepath.Join("/var/log/cronx", "backup", "42.log"); runPath != want {
		t.Errorf("RunPath() = %q, want %q", runPath, want)
	}
	if want := filepath.Join("/var/log/cronx", "cronx.log"); schedulerPath != want {
		t.Errorf("SchedulerPath() = %q, want %q", schedulerPath, want)
	}
}

func TestCreateRunFileCreatesTheDirectoriesAndTheFile(t *testing.T) {
	// SETUP
	layout := logx.NewLayout(filepath.Join(t.TempDir(), "logs"))
	const contents = "standard output of the run\n"

	// EXERCISE
	file, err := layout.CreateRunFile("backup", 7)
	if err != nil {
		t.Fatalf("CreateRunFile() returned an unexpected error: %v", err)
	}
	if _, err := file.WriteString(contents); err != nil {
		t.Fatalf("writing to the run log: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("closing the run log: %v", err)
	}

	// VERIFY
	read, err := os.ReadFile(layout.RunPath("backup", 7))
	if err != nil {
		t.Fatalf("reading back the run log: %v", err)
	}
	if string(read) != contents {
		t.Errorf("run log = %q, want %q", string(read), contents)
	}
}

func TestCreateRunFileReplacesPreviousContent(t *testing.T) {
	// SETUP
	layout := logx.NewLayout(filepath.Join(t.TempDir(), "logs"))
	first, err := layout.CreateRunFile("backup", 7)
	if err != nil {
		t.Fatalf("CreateRunFile() returned an unexpected error: %v", err)
	}
	if _, err := first.WriteString("stale output"); err != nil {
		t.Fatalf("writing to the run log: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("closing the run log: %v", err)
	}

	// EXERCISE
	second, err := layout.CreateRunFile("backup", 7)
	if err != nil {
		t.Fatalf("CreateRunFile() returned an unexpected error: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("closing the run log: %v", err)
	}

	// VERIFY
	read, err := os.ReadFile(layout.RunPath("backup", 7))
	if err != nil {
		t.Fatalf("reading back the run log: %v", err)
	}
	if len(read) != 0 {
		t.Errorf("run log = %q, want it to be empty", string(read))
	}
}

func TestLogFilesAreOnlyReadableByTheirOwner(t *testing.T) {
	// SETUP
	layout := logx.NewLayout(filepath.Join(t.TempDir(), "logs"))

	// EXERCISE
	file, err := layout.CreateRunFile("backup", 7)
	if err != nil {
		t.Fatalf("CreateRunFile() returned an unexpected error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("closing the run log: %v", err)
	}

	// VERIFY
	info, err := os.Stat(layout.RunPath("backup", 7))
	if err != nil {
		t.Fatalf("inspecting the run log: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("run log permissions = %v, want %v", got, want)
	}
}

func TestOpenSchedulerLogAppends(t *testing.T) {
	// SETUP
	layout := logx.NewLayout(filepath.Join(t.TempDir(), "logs"))
	first, err := layout.OpenSchedulerLog()
	if err != nil {
		t.Fatalf("OpenSchedulerLog() returned an unexpected error: %v", err)
	}
	if _, err := first.WriteString("first line\n"); err != nil {
		t.Fatalf("writing to the scheduler log: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("closing the scheduler log: %v", err)
	}

	// EXERCISE
	second, err := layout.OpenSchedulerLog()
	if err != nil {
		t.Fatalf("OpenSchedulerLog() returned an unexpected error: %v", err)
	}
	if _, err := second.WriteString("second line\n"); err != nil {
		t.Fatalf("writing to the scheduler log: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("closing the scheduler log: %v", err)
	}

	// VERIFY
	read, err := os.ReadFile(layout.SchedulerPath())
	if err != nil {
		t.Fatalf("reading back the scheduler log: %v", err)
	}
	if want := "first line\nsecond line\n"; string(read) != want {
		t.Errorf("scheduler log = %q, want %q", string(read), want)
	}
}
