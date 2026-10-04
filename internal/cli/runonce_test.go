package cli_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The run-once tests execute the test binary itself, so no shell is involved.
const (
	helperMarker = "CRONX_TEST_HELPER"
	helperMode   = "CRONX_TEST_MODE"
	helperExit   = "CRONX_TEST_EXIT"
)

// printedByTheJob is what the helper process writes to its standard output, so
// that the tests can find it in the log file of the run.
const printedByTheJob = "printed by the job"

func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperMarker) != "1" {
		return
	}
	switch os.Getenv(helperMode) {
	case "ok":
		// Nothing to do.
	case "print":
		fmt.Println(printedByTheJob)
	case "fail":
		code, err := strconv.Atoi(os.Getenv(helperExit))
		if err != nil {
			os.Exit(2)
		}
		os.Exit(code)
	}
	os.Exit(0)
}

// runOnceConfig builds a configuration whose single job runs the test binary in
// the given helper mode.
func runOnceConfig(t *testing.T, mode, exitCode string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test executable: %v", err)
	}

	return fmt.Sprintf(`
[jobs.hello]
schedule = "* * * * *"
command = %q
args = ["-test.run=TestHelperProcess"]
env = { %s = "1", %s = %q, %s = %q }
`,
		executable,
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
