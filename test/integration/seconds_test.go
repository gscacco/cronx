package integration_test

import (
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/store"
)

// A schedule with a seconds field fires more than once a minute, which no
// five-field expression can do. This test watches the real scheduler produce two
// runs of the same job within seconds of each other, and stops as soon as it
// has: the job it runs is one of the shortest, because every further run it
// waits through is a run nobody asked for.
//
// It waits for seconds rather than for a minute boundary, so it is cheap enough
// to run in the fast suite (go test -short) as well.
func TestASecondsFieldRunsMoreThanOnceAMinute(t *testing.T) {
	t.Parallel()

	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("seconds.toml")
	scheduler := environment.startScheduler(configPath)

	// EXERCISE: wait for the first run, then for a second one, which is
	// seconds away rather than a minute away. What is waited for is a
	// snapshot of the history with a finished second run in it, not two
	// recorded runs read once more afterwards: a run is written down before
	// the job is executed, so a job that fires every other second has its
	// next run in progress by the time a second read reaches the history.
	environment.waitForRun("ticks", job.StatusSucceeded, activationBudget)
	runs := environment.waitForRuns("ticks", secondsBudget, func(runs []store.Run) bool {
		return len(runs) >= 2 && !runs[0].FinishedAt.IsZero()
	})

	// VERIFY
	if !scheduler.running() {
		t.Fatalf("the scheduler exited on its own\nstdout:\n%s\nstderr:\n%s",
			scheduler.stdout(), scheduler.stderr())
	}

	// Both runs really happened: the job appended its mark to the file.
	if marks := environment.lines(environment.path("ticks")); len(marks) < 2 {
		t.Errorf("the job left %d marks, want one per run", len(marks))
	}

	// The two runs are seconds apart, which is what the seconds field asks
	// for: the finest gap a five-field expression can produce is a minute.
	latest, previous := runs[0], runs[1]
	if latest.Status != job.StatusSucceeded || previous.Status != job.StatusSucceeded {
		t.Errorf("the last two runs are recorded as %q and %q, want both %q",
			latest.Status, previous.Status, job.StatusSucceeded)
	}
	gap := latest.StartedAt.Sub(previous.StartedAt)
	if gap <= 0 || gap >= time.Minute {
		t.Errorf("the last two runs are %s apart, want seconds apart and not a whole minute",
			gap)
	}

	// The history shows what the schedule asks for, and `cronx list` reports
	// the expression as it was written.
	listed := environment.runOK("list", "--config", configPath)
	if row := listRowFor(t, listed.stdout, "ticks"); !strings.Contains(row, "*/2 * * * * *") {
		t.Errorf("list shows the job as %q, want the seconds field as it was written", row)
	}
}
