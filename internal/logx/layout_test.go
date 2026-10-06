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

func TestTheSchedulerLogFollowsTheDocumentedLayout(t *testing.T) {
	// SETUP
	layout := logx.NewLayout("/var/log/cronx")

	// EXERCISE
	schedulerPath := layout.SchedulerPath()

	// VERIFY
	if want := filepath.Join("/var/log/cronx", "cronx.log"); schedulerPath != want {
		t.Errorf("SchedulerPath() = %q, want %q", schedulerPath, want)
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
