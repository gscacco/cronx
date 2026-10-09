package scheduler

import (
	"context"
	"testing"
	"time"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

// These tests call the dispatch loop and the start of a run directly, because
// the rule they pin is what the scheduler does with a history that is longer
// than the configuration keeps: it trims it when it starts, and again for a job
// whose trigger arrives.

// seedInternalRuns records n finished runs of a job, an hour apart, and returns
// their identifiers, oldest first.
func seedInternalRuns(t *testing.T, persistent *store.Store, name string, n int) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, 0, n)
	for index := 0; index < n; index++ {
		at := time.Now().Add(-time.Duration(n-index) * time.Hour)
		id, err := persistent.StartRun(ctx, name, 1, at)
		if err != nil {
			t.Fatalf("StartRun() returned an unexpected error: %v", err)
		}
		if err := persistent.FinishRun(ctx, id, store.Finish{
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

func TestTheSchedulerPrunesTheHistoryWhenItStarts(t *testing.T) {
	// SETUP: a history longer than the configuration keeps, and a job that
	// never runs, so that nothing is added while the scheduler is up.
	definition := internalJob(t, "backup", job.OverlapSkip)
	definition.Schedule = "0 0 31 4 *"
	subject, persistent := buildInternalSchedulerWith(t, config.Storage{MaxRuns: 2}, 1, definition)
	seedInternalRuns(t, persistent, definition.Name, 5)

	// EXERCISE: the scheduler starts and is stopped again.
	runCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	if err := subject.Run(runCtx); err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}

	// VERIFY
	runs := runsOf(t, persistent, definition.Name)
	if len(runs) != 2 {
		t.Errorf("the history holds %d runs after the scheduler started, want the 2 the configuration keeps",
			len(runs))
	}
}

func TestTheSchedulerPrunesTheHistoryWhenATriggerArrives(t *testing.T) {
	// SETUP: a history longer than the configuration keeps, and a job that is
	// due now. Its overlap policy has it skip while a run of its own is in
	// progress, which the test holds, so the trigger is recorded without a
	// process being started and nothing has to be waited for.
	definition := internalJob(t, "backup", job.OverlapSkip)
	subject, persistent := buildInternalSchedulerWith(t, config.Storage{MaxRuns: 2}, 1, definition)
	ids := seedInternalRuns(t, persistent, definition.Name, 5)

	subject.locks[definition.Name].Lock()
	defer subject.locks[definition.Name].Unlock()

	// EXERCISE: a trigger arrives.
	at := time.Now()
	subject.next[definition.Name] = at
	subject.dispatch(context.Background(), at)

	// VERIFY: the runs the configuration does not keep are gone, whichever
	// run the trigger itself recorded.
	for _, run := range runsOf(t, persistent, definition.Name) {
		if run.ID <= ids[2] {
			t.Errorf("run %d is still in the history, want it pruned: the configuration keeps the newest two",
				run.ID)
		}
	}
}
