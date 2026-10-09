package integration_test

import "testing"

// The retention tests check that the history does not grow without bound: the
// scheduler trims it to the number of runs the configuration keeps.

func TestTheHistoryIsPrunedWhenTheSchedulerStarts(t *testing.T) {
	// SETUP: five runs recorded by hand, and a configuration that keeps two.
	environment := newEnvironment(t)
	configPath := environment.configure("retention.toml")
	for run := 0; run < 5; run++ {
		environment.runOK("run-once", "talker", "--config", configPath)
	}
	if runs := environment.runs("talker"); len(runs) != 5 {
		t.Fatalf("the history holds %d runs before the scheduler starts, want 5", len(runs))
	}

	// EXERCISE: the scheduler starts, which is when it trims the history,
	// and is then stopped again.
	running := environment.startScheduler(configPath)
	running.stop()

	// VERIFY: the two newest runs are left, in the database and in what the
	// command reports.
	runs := environment.runs("talker")
	if len(runs) != 2 {
		t.Fatalf("the history holds %d runs after the scheduler started, want the 2 the configuration keeps\n%s",
			len(runs), environment.describe())
	}
	if runs[0].Status == "" || runs[1].Status == "" {
		t.Errorf("the pruned runs have no status: %v and %v", runs[0].Status, runs[1].Status)
	}

	history := environment.runOK("history", "--config", configPath)
	if rows := historyRows(t, history.stdout, "talker"); len(rows) != 2 {
		t.Errorf("cronx history shows %d rows of the job, want 2\n%s", len(rows), history.stdout)
	}
}
