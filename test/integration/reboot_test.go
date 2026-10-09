package integration_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/job"
)

// A job that runs when the scheduler starts is triggered by the start itself
// and not by a cron expression, so this test waits for nothing on the clock: it
// is the quickest way to watch the real scheduler produce a run.
func TestARebootJobRunsWhenTheSchedulerStarts(t *testing.T) {
	t.Parallel()

	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("reboot.toml")
	scheduler := environment.startScheduler(configPath)

	// EXERCISE: wait for the run, which owes nothing to a minute boundary.
	run := environment.waitForRun("reboot", job.StatusSucceeded, settleBudget)

	// VERIFY
	if !scheduler.running() {
		t.Fatalf("the scheduler exited on its own\nstdout:\n%s\nstderr:\n%s",
			scheduler.stdout(), scheduler.stderr())
	}

	// The job produced its observable result.
	if content := environment.readFile(environment.path("reboot")); content != "ran" {
		t.Errorf("the job wrote %q, want %q", content, "ran")
	}

	// The run is recorded like any other, with its output in the shared log.
	if run.ExitCode == nil || *run.ExitCode != 0 {
		t.Errorf("the run recorded the exit code %v, want 0", run.ExitCode)
	}
	if run.LogPath == "" {
		t.Fatal("the run was recorded without a log file")
	}
	if output := environment.readFile(run.LogPath); !strings.Contains(output, "the reboot job ran") {
		t.Errorf("the log of the run holds %q, want the output of the job", output)
	}

	// The job ran once: a start happens once, however long the scheduler runs
	// afterwards.
	if runs := environment.runs("reboot"); len(runs) != 1 {
		t.Errorf("the job has %d runs, want exactly one, from the start", len(runs))
	}

	// The commands say what the job is waiting for, which is a scheduler that
	// is not running yet rather than an instant.
	listed := environment.runOK("list", "--config", configPath)
	if row := listRowFor(t, listed.stdout, "reboot"); !strings.Contains(row, "at startup") {
		t.Errorf("list shows reboot as %q, want it to say %q", row, "at startup")
	}
}

// listRowFor returns the line of the list output that describes a job.
func listRowFor(t *testing.T, output, name string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == name {
			return line
		}
	}
	t.Fatalf("list output = %q, want a row for the job %q", output, name)
	return ""
}
