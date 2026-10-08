package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

// These tests live in the package rather than in store_test, because the race
// they cover lives between the version check and the migration that records it:
// a second process is a matter of timing, whereas the state the second process
// finds is not.

func TestApplyingAMigrationThatIsAlreadyAppliedChangesNothing(t *testing.T) {
	// SETUP: a database that has just been migrated.
	path := filepath.Join(t.TempDir(), "state.db")
	subject, err := Open(path)
	if err != nil {
		t.Fatalf("Open() returned an unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = subject.Close() })
	ctx := context.Background()

	// EXERCISE: the very migration that produced the schema, applied again,
	// which is what the second of two processes does when it decides to migrate
	// a database the first one has already migrated.
	err = subject.applyMigration(ctx, 1)

	// VERIFY
	if err != nil {
		t.Fatalf("applyMigration() on a database that already holds the schema returned %v, want nil", err)
	}
	version, err := subject.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion() returned an unexpected error: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Errorf("SchemaVersion() = %d, want %d", version, CurrentSchemaVersion)
	}
}

func TestOpeningANewDatabaseFromManyProcessesAtOnceAlwaysComesUp(t *testing.T) {
	// SETUP: one database that does not exist yet, and several openers that
	// wait for the same instant so that they race for it exactly as two cronx
	// processes started together would.
	path := filepath.Join(t.TempDir(), "state.db")
	const openers = 8
	start := make(chan struct{})

	var (
		waiting sync.WaitGroup
		mu      sync.Mutex
		failed  []error
	)
	waiting.Add(openers)

	// EXERCISE
	for index := 0; index < openers; index++ {
		go func() {
			defer waiting.Done()
			<-start
			subject, err := Open(path)
			if err != nil {
				mu.Lock()
				failed = append(failed, err)
				mu.Unlock()
				return
			}
			_ = subject.Close()
		}()
	}
	close(start)
	waiting.Wait()

	// VERIFY
	if len(failed) > 0 {
		t.Fatalf("%d of the %d opens of a brand new database failed, want none: %v",
			len(failed), openers, failed)
	}
}
