package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gscacco.com/cronx/internal/store"
)

// newStore returns a private in-memory store that is closed when the test ends.
func newStore(t *testing.T) *store.Store {
	t.Helper()
	subject, err := store.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory() returned an unexpected error: %v", err)
	}
	t.Cleanup(func() {
		if err := subject.Close(); err != nil {
			t.Errorf("closing the store: %v", err)
		}
	})
	return subject
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parsing time %q: %v", value, err)
	}
	return parsed
}

func intPointer(value int) *int {
	return &value
}

func TestOpenMemoryAppliesTheSchema(t *testing.T) {
	// SETUP
	subject := newStore(t)

	// EXERCISE
	version, err := subject.SchemaVersion(context.Background())

	// VERIFY
	if err != nil {
		t.Fatalf("SchemaVersion() returned an unexpected error: %v", err)
	}
	if version != store.CurrentSchemaVersion {
		t.Errorf("SchemaVersion() = %d, want %d", version, store.CurrentSchemaVersion)
	}
}

func TestOpenCreatesMissingDirectories(t *testing.T) {
	// SETUP
	path := filepath.Join(t.TempDir(), "nested", "cronx", "state.db")

	// EXERCISE
	subject, err := store.Open(path)

	// VERIFY
	if err != nil {
		t.Fatalf("Open() returned an unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = subject.Close() })
}

func TestOpenKeepsTheDataWhenReopened(t *testing.T) {
	// SETUP
	path := filepath.Join(t.TempDir(), "state.db")
	ctx := context.Background()

	first, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() returned an unexpected error: %v", err)
	}
	if _, err := first.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T00:00:00Z")); err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() returned an unexpected error: %v", err)
	}

	// EXERCISE
	second, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopening the store returned an unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	runs, err := second.Runs(ctx, "backup", 10)

	// VERIFY
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(Runs()) = %d, want the run recorded before the store was closed", len(runs))
	}

	version, err := second.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion() returned an unexpected error: %v", err)
	}
	if version != store.CurrentSchemaVersion {
		t.Errorf("SchemaVersion() = %d, want %d after reopening", version, store.CurrentSchemaVersion)
	}
}
