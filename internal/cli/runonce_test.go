package cli_test

import (
	"fmt"
	"strings"
	"testing"
)

// runOnceConfig builds a configuration whose single job runs the test binary in
// the given helper mode.
func runOnceConfig(t *testing.T, mode, exitCode string) string {
	t.Helper()

	return fmt.Sprintf(`
[jobs.hello]
schedule = "* * * * *"
command = %q
args = ["-test.run=TestHelperProcess"]
env = { %s = "1", %s = %q, %s = %q }
`,
		testExecutable(t),
		helperMarker, helperMode, mode, helperExit, exitCode)
}

func TestRunOnceExecutesAJob(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, runOnceConfig(t, "ok", "0"))

	// EXERCISE
	output, err := runCLI(t, "run-once", "hello", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("run-once returned an unexpected error: %v\noutput: %s", err, output)
	}
	if !strings.Contains(output, "succeeded") {
		t.Errorf("run-once output = %q, want it to report the outcome", output)
	}

	history, err := runCLI(t, "history", "--config", path)
	if err != nil {
		t.Fatalf("history returned an unexpected error: %v", err)
	}
	if !strings.Contains(history, "succeeded") {
		t.Errorf("history output = %q, want the run to have been recorded", history)
	}
}

func TestRunOnceReportsAFailingJob(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, runOnceConfig(t, "fail", "3"))

	// EXERCISE
	output, err := runCLI(t, "run-once", "hello", "--config", path)

	// VERIFY
	if err == nil {
		t.Fatalf("run-once succeeded, want an error for a job that failed\noutput: %s", output)
	}
	if !strings.Contains(output, "failed") {
		t.Errorf("run-once output = %q, want it to report the outcome", output)
	}
}

func TestRunOnceRejectsAnUnknownJob(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, runOnceConfig(t, "ok", "0"))

	// EXERCISE
	_, err := runCLI(t, "run-once", "missing", "--config", path)

	// VERIFY
	if err == nil {
		t.Fatalf("run-once succeeded, want an error for a job that does not exist")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("run-once error = %q, want it to name the job", err.Error())
	}
}
