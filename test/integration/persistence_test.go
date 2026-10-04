package integration_test

import (
	"os"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

// The persistence tests check what survives a restart: the history, the state
// database itself, and the runs a process that was killed left behind.

func TestTheHistorySurvivesARestart(t *testing.T) {
	t.Parallel()

	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("persistence.toml")

	first := environment.startScheduler(configPath)
	firstRun := environment.waitForRun("tick", job.StatusSucceeded, activationBudget)
	if code := first.stop(); code != 0 {
		t.Fatalf("the first scheduler exited with status %d, want success\nstdout:\n%s\nstderr:\n%s",
			code, first.stdout(), first.stderr())
	}
	stateBefore := environment.stateInfo()

	// EXERCISE: start a second scheduler on the same configuration and the same
	// state database.
	second := environment.startScheduler(configPath)
	secondRun := environment.waitForRunAfter("tick", firstRun.ID, job.StatusSucceeded, activationBudget)
	if code := second.stop(); code != 0 {
		t.Errorf("the second scheduler exited with status %d, want success", code)
	}

	// VERIFY: the database was opened again, not rebuilt.
	if stateAfter := environment.stateInfo(); !os.SameFile(stateBefore, stateAfter) {
		t.Error("the state database was replaced by the restart, want the same file to be used again")
	}
	if version := environment.schemaVersion(); version != store.CurrentSchemaVersion {
		t.Errorf("the state database holds schema version %d, want %d: a restart must not rebuild it",
			version, store.CurrentSchemaVersion)
	}

	// The run recorded before the restart is still there, unchanged.
	kept := false
	for _, run := range environment.runs("tick") {
		if run.ID != firstRun.ID {
			continue
		}
		kept = true
		if run.Status != firstRun.Status || run.Attempt != firstRun.Attempt || run.LogPath != firstRun.LogPath {
			t.Errorf("run %d is now %q (attempt %d, log %s), want the recorded %q (attempt %d, log %s)",
				run.ID, run.Status, run.Attempt, run.LogPath,
				firstRun.Status, firstRun.Attempt, firstRun.LogPath)
		}
		if !run.FinishedAt.Equal(firstRun.FinishedAt) {
			t.Errorf("run %d finished at %s after the restart, want the recorded %s",
				run.ID, run.FinishedAt, firstRun.FinishedAt)
		}
	}
	if !kept {
		t.Errorf("the run recorded before the restart (run %d) is missing from the history", firstRun.ID)
	}
	if secondRun.ID <= firstRun.ID {
		t.Errorf("the run recorded after the restart (run %d) is not newer than the one before it (run %d)",
			secondRun.ID, firstRun.ID)
	}

	// The commands read the same history, and the output of both runs is still
	// on disk.
	history := environment.runOK("history", "tick", "--config", configPath)
	if rows := historyRows(t, history.stdout, "tick"); len(rows) < 2 {
		t.Errorf("history lists %d runs after the restart, want at least the two that ran\n%s",
			len(rows), history.stdout)
	}
	for _, run := range []store.Run{firstRun, secondRun} {
		if _, err := os.Stat(run.LogPath); err != nil {
			t.Errorf("the log of run %d is missing after the restart: %v", run.ID, err)
		}
	}
}

func TestRunsLeftInProgressAreClosedOnTheNextStart(t *testing.T) {
	t.Parallel()

	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("stale.toml")

	first := environment.startScheduler(configPath)
	running := environment.waitForRun("long", job.StatusRunning, activationBudget)
	report := environment.waitForReport(environment.path("long.json"), settleBudget)
	// The job outlives the scheduler it belongs to, which is about to be killed
	// without being given the chance to stop it.
	t.Cleanup(func() { removeProcess(report.PID) })

	// EXERCISE: kill the scheduler, then start another one on the same state.
	first.kill()
	second := environment.startScheduler(configPath)
	closed := environment.waitForRunWhere("long", settleBudget, func(run store.Run) bool {
		return run.ID == running.ID && run.Status == job.StatusFailed
	})

	// VERIFY: the run that could not be observed any more is explained instead
	// of being left in progress for ever.
	if !strings.Contains(closed.Error, "in progress") {
		t.Errorf("run %d was closed with %q, want it to explain that the scheduler stopped while it was in progress",
			closed.ID, closed.Error)
	}
	if !closed.FinishedAt.After(running.StartedAt) {
		t.Errorf("run %d was closed at %s, want a time after it started at %s",
			closed.ID, closed.FinishedAt, running.StartedAt)
	}
	if log := environment.readFile(environment.schedulerLog()); !strings.Contains(log, "closed runs left in progress") {
		t.Errorf("the scheduler log holds %q, want it to record that runs left in progress were closed", log)
	}

	if code := second.stop(); code != 0 {
		t.Errorf("the second scheduler exited with status %d, want success", code)
	}
	if count := environment.runningRuns("long"); count != 0 {
		t.Errorf("%d runs are still recorded as in progress, want none once a scheduler has looked at them", count)
	}
}
