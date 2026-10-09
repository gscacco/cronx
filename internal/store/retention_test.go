package store_test

import (
	"context"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

// seedRuns records n finished runs of a job, an hour apart, and returns their
// identifiers, oldest first.
func seedRuns(t *testing.T, subject *store.Store, name string, n int) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, 0, n)
	for index := 0; index < n; index++ {
		at := mustTime(t, "2026-01-01T00:00:00Z").Add(time.Duration(index) * time.Hour)
		id, err := subject.StartRun(ctx, name, 1, at)
		if err != nil {
			t.Fatalf("StartRun() returned an unexpected error: %v", err)
		}
		if err := subject.FinishRun(ctx, id, store.Finish{
			Status:     job.StatusSucceeded,
			FinishedAt: at.Add(time.Second),
			Duration:   time.Second,
		}); err != nil {
			t.Fatalf("FinishRun() returned an unexpected error: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestPruneRunsKeepsTheNewestRuns(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	ids := seedRuns(t, subject, "backup", 5)

	// EXERCISE
	deleted, err := subject.PruneRuns(ctx, "backup", 2)

	// VERIFY: the two newest runs are what is left.
	if err != nil {
		t.Fatalf("PruneRuns() returned an unexpected error: %v", err)
	}
	if want := int64(3); deleted != want {
		t.Errorf("PruneRuns() = %d, want %d", deleted, want)
	}
	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("the history holds %d runs, want 2", len(runs))
	}
	if runs[0].ID != ids[4] || runs[1].ID != ids[3] {
		t.Errorf("the history holds runs %d and %d, want the two newest (%d and %d)",
			runs[0].ID, runs[1].ID, ids[4], ids[3])
	}
}

func TestPruneRunsLeavesTheOtherJobsAlone(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	seedRuns(t, subject, "backup", 3)
	seedRuns(t, subject, "cleanup", 3)

	// EXERCISE
	if _, err := subject.PruneRuns(ctx, "backup", 1); err != nil {
		t.Fatalf("PruneRuns() returned an unexpected error: %v", err)
	}

	// VERIFY
	for name, want := range map[string]int{"backup": 1, "cleanup": 3} {
		runs, err := subject.Runs(ctx, name, 10)
		if err != nil {
			t.Fatalf("Runs(%s) returned an unexpected error: %v", name, err)
		}
		if len(runs) != want {
			t.Errorf("the history of %q holds %d runs, want %d", name, len(runs), want)
		}
	}
}

func TestPruneRunsNeverDeletesARunInProgress(t *testing.T) {
	// SETUP: four finished runs, and one that is still going.
	subject := newStore(t)
	ctx := context.Background()
	seedRuns(t, subject, "backup", 4)
	id, err := subject.StartRun(ctx, "backup", 1, mustTime(t, "2026-01-01T05:00:00Z"))
	if err != nil {
		t.Fatalf("StartRun() returned an unexpected error: %v", err)
	}

	// EXERCISE
	if _, err := subject.PruneRuns(ctx, "backup", 2); err != nil {
		t.Fatalf("PruneRuns() returned an unexpected error: %v", err)
	}

	// VERIFY: the run in progress is not history yet, so it is kept whatever
	// its age, together with the two newest finished runs.
	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("the history holds %d runs, want the two newest and the one in progress", len(runs))
	}
	if runs[0].ID != id {
		t.Errorf("the newest run is %d, want the run in progress (%d)", runs[0].ID, id)
	}
	if runs[0].Status != job.StatusRunning {
		t.Errorf("the newest run is %q, want %q", runs[0].Status, job.StatusRunning)
	}
}

func TestPruneRunsWithNothingToKeepDeletesNothing(t *testing.T) {
	// SETUP
	subject := newStore(t)
	ctx := context.Background()
	seedRuns(t, subject, "backup", 3)

	// EXERCISE: a keep that says nothing is asked for must not empty the
	// history, whatever a caller passes by mistake.
	for _, keep := range []int{0, -1} {
		deleted, err := subject.PruneRuns(ctx, "backup", keep)

		// VERIFY
		if err != nil {
			t.Fatalf("PruneRuns(%d) returned an unexpected error: %v", keep, err)
		}
		if deleted != 0 {
			t.Errorf("PruneRuns(%d) = %d, want 0", keep, deleted)
		}
	}
	runs, err := subject.Runs(ctx, "backup", 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("the history holds %d runs, want the three that were there", len(runs))
	}
}
