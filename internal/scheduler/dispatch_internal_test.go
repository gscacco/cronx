package scheduler

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/logx"
	"gscacco.com/cronx/internal/runner"
	"gscacco.com/cronx/internal/store"
)

// These tests call the dispatch loop directly, because the rule they pin is the
// order of two things that loop does: a trigger is judged by the overlap policy
// of its job when it arrives, before the job waits for a free slot. Reaching
// that order through Run would take two real minute boundaries, which is what
// test/integration does with the very same configuration.

// The test binary re-executes itself as the job, exactly as the tests of the
// external package do.
const (
	internalHelperMarker = "CRONX_TEST_HELPER"
	internalHelperMode   = "CRONX_TEST_MODE"
	internalHelperSleep  = "CRONX_TEST_SLEEP"
)

// dispatchSleep is long enough for the run it starts to be observed while the
// next trigger arrives.
const dispatchSleep = "400ms"

func TestATriggerIsSkippedBeforeTheJobWaitsForASlot(t *testing.T) {
	// SETUP: one slot, and a job that outlives the interval between the two
	// triggers dispatched below.
	definition := internalJob(t, "backup", job.OverlapSkip)
	subject, persistent := buildInternalScheduler(t, 1, definition)
	ctx := context.Background()

	at := time.Now()
	subject.next[definition.Name] = at
	subject.dispatch(ctx, at)
	waitForOneRunInProgress(t, persistent, definition.Name)

	// EXERCISE: the next trigger arrives while the run is still in progress.
	subject.next[definition.Name] = at
	subject.dispatch(ctx, at)

	// VERIFY: the trigger was judged when it arrived, so it is already
	// recorded as skipped while the run it overlaps is still going, and the
	// job was not started a second time.
	runs := runsOf(t, persistent, definition.Name)
	if len(runs) != 2 {
		t.Fatalf("the job has %d recorded runs, want the run in progress and the trigger that overlaps it",
			len(runs))
	}
	if runs[0].Status != job.StatusSkipped {
		t.Errorf("the trigger that arrived while the only slot was taken is recorded as %q, want %q: the overlap policy of a job is applied when the trigger arrives",
			runs[0].Status, job.StatusSkipped)
	}
	if runs[0].Attempt != 0 {
		t.Errorf("the skipped trigger is recorded as attempt %d, want 0 because no process was attempted",
			runs[0].Attempt)
	}
	if !strings.Contains(runs[0].Error, "overlap") {
		t.Errorf("the skipped trigger is explained as %q, want the overlap policy named", runs[0].Error)
	}
	if running := runningRunsOf(t, persistent, definition.Name); running != 1 {
		t.Errorf("%d runs of the job are in progress, want only the first one", running)
	}
}

func TestAQueuedTriggerWaitsForTheRunInProgress(t *testing.T) {
	// SETUP: one slot, and a job whose triggers wait instead of being dropped.
	definition := internalJob(t, "backup", job.OverlapQueue)
	subject, persistent := buildInternalScheduler(t, 1, definition)
	ctx := context.Background()

	at := time.Now()
	subject.next[definition.Name] = at
	subject.dispatch(ctx, at)
	waitForOneRunInProgress(t, persistent, definition.Name)

	// EXERCISE: the next trigger arrives while the run is still in progress.
	subject.next[definition.Name] = at
	subject.dispatch(ctx, at)

	// VERIFY: it is recorded, and it starts only once the run in progress has
	// finished, so the job is never executed twice at the same time.
	eventually(t, "the queued trigger to have run", func() bool {
		runs := runsOf(t, persistent, definition.Name)
		return len(runs) == 2 &&
			runs[0].Status == job.StatusSucceeded &&
			runs[1].Status == job.StatusSucceeded
	})
	runs := runsOf(t, persistent, definition.Name)
	if runs[0].StartedAt.Before(runs[1].FinishedAt) {
		t.Errorf("the queued run started at %s, before the run it follows finished at %s",
			runs[0].StartedAt, runs[1].FinishedAt)
	}
}

// internalJob builds a job that runs this test binary as its own helper
// process, sleeping for the time the other tests of the package wait through.
func internalJob(t *testing.T, name string, overlap job.OverlapPolicy) job.Job {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test executable: %v", err)
	}
	return job.Job{
		Name:     name,
		Schedule: "* * * * *",
		Command:  executable,
		Args:     []string{"-test.run=TestHelperProcess"},
		Overlap:  overlap,
		Env: map[string]string{
			internalHelperMarker: "1",
			internalHelperMode:   "sleep",
			internalHelperSleep:  dispatchSleep,
		},
	}
}

// buildInternalScheduler wires a scheduler around an in-memory store and a run
// log in a temporary directory, for the tests of this package.
func buildInternalScheduler(t *testing.T, parallel int, definitions ...job.Job) (*Scheduler, *store.Store) {
	t.Helper()
	return buildInternalSchedulerWith(t, config.Storage{}, parallel, definitions...)
}

// buildInternalSchedulerWith is buildInternalScheduler with the [storage]
// settings given, so that a test can ask for a history that is pruned.
func buildInternalSchedulerWith(t *testing.T, storage config.Storage, parallel int, definitions ...job.Job) (*Scheduler, *store.Store) {
	t.Helper()

	persistent, err := store.OpenMemory()
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	t.Cleanup(func() { _ = persistent.Close() })

	clk := clock.System{}
	runs := logx.Open(filepath.Join(t.TempDir(), "logs", "runs.log"), clk, logx.Rotation{})
	t.Cleanup(func() { _ = runs.Close() })

	logger, err := logx.NewLogger(io.Discard, "error")
	if err != nil {
		t.Fatalf("building the logger: %v", err)
	}

	configured := make(map[string]job.Job, len(definitions))
	for _, definition := range definitions {
		configured[definition.Name] = definition
	}

	built, err := New(Options{
		Config: config.Config{
			Scheduler: config.Scheduler{Timezone: "Local", MaxParallelJobs: parallel},
			Logging:   config.Logging{Level: "error"},
			Storage:   storage,
			Jobs:      configured,
		},
		Store:  persistent,
		Logs:   runs,
		Runner: runner.New(clk),
		Clock:  clk,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("building the scheduler: %v", err)
	}
	return built, persistent
}

// waitForOneRunInProgress waits until the job has exactly one run going, which
// is also the moment its overlap gate is held.
func waitForOneRunInProgress(t *testing.T, persistent *store.Store, name string) {
	t.Helper()
	eventually(t, "the run to be in progress", func() bool {
		return runningRunsOf(t, persistent, name) == 1
	})
}

// eventually polls a condition until it holds, or fails the test.
func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not happen within five seconds", what)
}

// runsOf returns the recorded runs of a job, newest first.
func runsOf(t *testing.T, persistent *store.Store, name string) []store.Run {
	t.Helper()
	runs, err := persistent.Runs(context.Background(), name, 10)
	if err != nil {
		t.Fatalf("Runs() returned an unexpected error: %v", err)
	}
	return runs
}

// runningRunsOf returns how many runs of a job are in progress.
func runningRunsOf(t *testing.T, persistent *store.Store, name string) int {
	t.Helper()
	running, err := persistent.RunningRuns(context.Background(), name)
	if err != nil {
		t.Fatalf("RunningRuns() returned an unexpected error: %v", err)
	}
	return running
}
