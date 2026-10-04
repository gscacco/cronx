package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The workflow tests follow a configuration file from start to finish: the file
// is rendered, validated, listed, used to run jobs, and only then inspected
// through the state and the logs the runs have left behind.

// workflowFixture is the configuration file the workflow tests are driven by.
// Unlike the reference examples in examples/configs, it is executed.
const workflowFixture = "workflow.toml"

// The placeholders the fixture uses for the paths only the tests know.
const (
	executablePlaceholder       = "@TEST_EXECUTABLE@"
	workingDirectoryPlaceholder = "@TEST_WORKDIR@"
)

// newWorkflow writes the fixture into a fresh home and returns the home and the
// path of the configuration file that was written there.
func newWorkflow(t *testing.T) (home, path string) {
	t.Helper()
	home = newTestHome(t)

	workingDirectory := filepath.Join(home, "work")
	if err := os.MkdirAll(workingDirectory, 0o700); err != nil {
		t.Fatalf("creating the working directory: %v", err)
	}

	contents := strings.NewReplacer(
		executablePlaceholder, testExecutable(t),
		workingDirectoryPlaceholder, workingDirectory,
	).Replace(readFixture(t, workflowFixture))

	return home, writeTestConfig(t, home, contents)
}

// physicalPath returns a path as the processes of the machine report it, so
// that the comparison holds also where a temporary directory is reached through
// a symbolic link.
func physicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolving %s: %v", path, err)
	}
	return resolved
}

func TestWorkflowRunsAJobDescribedByAConfigurationFile(t *testing.T) {
	// SETUP
	home, path := newWorkflow(t)

	// EXERCISE
	if _, err := runCLI(t, "validate", "--config", path); err != nil {
		t.Fatalf("validate returned an unexpected error: %v", err)
	}

	listed, err := runCLI(t, "list", "--config", path)
	if err != nil {
		t.Fatalf("list returned an unexpected error: %v", err)
	}

	output, err := runCLI(t, "run-once", "report", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("run-once returned an unexpected error: %v\noutput: %s", err, output)
	}
	for _, name := range []string{"flaky", "report", "slow"} {
		if !strings.Contains(listed, name) {
			t.Errorf("list output = %q, want it to contain the job %q", listed, name)
		}
	}
	if !strings.Contains(output, `"succeeded"`) {
		t.Errorf("run-once output = %q, want it to report the outcome", output)
	}

	status, err := runCLI(t, "status", "--config", path)
	if err != nil {
		t.Fatalf("status returned an unexpected error: %v", err)
	}
	if !strings.Contains(status, "succeeded") {
		t.Errorf("status output = %q, want the outcome of the job that ran", status)
	}

	history, err := runCLI(t, "history", "report", "--config", path)
	if err != nil {
		t.Fatalf("history returned an unexpected error: %v", err)
	}
	if !strings.Contains(history, "report") {
		t.Errorf("history output = %q, want the run to have been recorded", history)
	}

	// The job saw the working directory the configuration asked for, the
	// variables the configuration declared and nothing else: the environment of
	// a job is the minimal one plus its own, so nothing of the environment of
	// the scheduler can leak into it.
	logs := runLogFiles(t, home, "report")
	if len(logs) != 1 {
		t.Fatalf("the run produced %d log files, want 1: %v", len(logs), logs)
	}
	log := readFile(t, logs[0])
	wants := []string{
		"cwd=" + physicalPath(t, filepath.Join(home, "work")),
		"message=hello from the fixture",
		"env=CRONX_TEST_HELPER,CRONX_TEST_MESSAGE,CRONX_TEST_MODE,PATH",
	}
	for _, want := range wants {
		if !strings.Contains(log, want) {
			t.Errorf("run log = %q, want it to contain %q", log, want)
		}
	}
}

func TestWorkflowRepeatsAJobThatKeepsFailing(t *testing.T) {
	// SETUP
	_, path := newWorkflow(t)

	// EXERCISE
	output, err := runCLI(t, "run-once", "flaky", "--config", path)

	// VERIFY
	if err == nil {
		t.Fatalf("run-once succeeded, want an error for a job that does not succeed")
	}
	if !strings.Contains(output, `"failed"`) {
		t.Errorf("run-once output = %q, want it to report the outcome", output)
	}

	history, err := runCLI(t, "history", "flaky", "--config", path)
	if err != nil {
		t.Fatalf("history returned an unexpected error: %v", err)
	}
	attempts := 0
	for _, line := range strings.Split(history, "\n") {
		if !strings.Contains(line, "flaky") {
			continue
		}
		attempts++
		if !strings.Contains(line, "failed") {
			t.Errorf("history line = %q, want the attempt to be reported as failed", line)
		}
		if !strings.HasSuffix(strings.TrimSpace(line), "3") {
			t.Errorf("history line = %q, want the exit code 3 of the attempt", line)
		}
	}
	if attempts != 3 {
		t.Errorf("history lists %d attempts, want 3 because the configuration retries twice\n%s",
			attempts, history)
	}

	status, err := runCLI(t, "status", "--config", path)
	if err != nil {
		t.Fatalf("status returned an unexpected error: %v", err)
	}
	if !strings.Contains(status, "failed") {
		t.Errorf("status output = %q, want the last outcome of the job", status)
	}
}

func TestWorkflowStopsAJobThatExceedsItsTimeout(t *testing.T) {
	// SETUP
	_, path := newWorkflow(t)

	// EXERCISE
	startedAt := time.Now()
	output, err := runCLI(t, "run-once", "slow", "--config", path)
	elapsed := time.Since(startedAt)

	// VERIFY
	if err == nil {
		t.Fatalf("run-once succeeded, want an error for a job that is stopped")
	}
	if !strings.Contains(output, `"timed_out"`) {
		t.Errorf("run-once output = %q, want it to report that the job timed out", output)
	}
	if elapsed >= helperSleep {
		t.Errorf("the run lasted %s, want it to end well before the %s the job sleeps for",
			elapsed, helperSleep)
	}

	status, err := runCLI(t, "status", "--config", path)
	if err != nil {
		t.Fatalf("status returned an unexpected error: %v", err)
	}
	if !strings.Contains(status, "timed_out") {
		t.Errorf("status output = %q, want the last outcome of the job", status)
	}
}
