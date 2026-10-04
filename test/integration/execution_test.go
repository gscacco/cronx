package integration_test

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/job"
)

// The execution tests check what the process a job starts actually receives.
// They use run-once, which is the same scheduler, the same store and the same
// runner as a scheduled run: only the trigger differs.

func TestCommandsAndArgumentsReachTheProcessVerbatim(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)

	// The working directory of the job holds files, so that a glob would have
	// something to expand into if the arguments went through a shell.
	glob := environment.makeDirectory("glob")
	environment.makeFile(environment.path("glob/one.txt"), "one")
	environment.makeFile(environment.path("glob/two.txt"), "two")
	configPath := environment.configure("arguments.toml")

	// EXERCISE
	environment.runOK("run-once", "arguments", "--config", configPath)
	recorded := environment.waitForReport(environment.path("arguments.json"), settleBudget)

	// VERIFY: the program is the one the configuration names, and it saw the
	// working directory it was given.
	if len(recorded.Argv) == 0 || recorded.Argv[0] != jobHelper {
		t.Errorf("the job was started as %v, want it to be %s", recorded.Argv, jobHelper)
	}
	if recorded.Dir != physicalPath(t, glob) {
		t.Errorf("the job ran in %q, want the configured working directory %q",
			recorded.Dir, physicalPath(t, glob))
	}

	// The payload is the one the configuration declares, argument by argument,
	// boundaries included. A shell would have split "two words" in two, expanded
	// "*" into the files above, run the command substitution, and executed what
	// followed the semicolon.
	want := []string{
		"plain",
		"two words",
		"  padded  ",
		"$(whoami)",
		"`id`",
		"; touch " + environment.path("shell-ran"),
		"*",
		"?",
		"--flag=value",
		"quote\"inside",
		"line\nbreak",
		"tab\tinside",
		"back\\slash",
		"",
	}
	if !slices.Equal(recorded.Payload, want) {
		t.Errorf("the job received the arguments\n%q\nwant\n%q", recorded.Payload, want)
	}
	if _, err := os.Stat(environment.path("shell-ran")); err == nil {
		t.Error("the job created the file named after the semicolon, want nothing after a semicolon to be executed")
	}
}

func TestTheJobRunsInTheWorkingDirectoryItWasGiven(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	requested := environment.makeDirectory("requested")
	configPath := environment.configure("working_directory.toml")

	// EXERCISE
	environment.runOK("run-once", "report", "--config", configPath)
	recorded := environment.waitForReport(environment.path("cwd.json"), settleBudget)

	// VERIFY
	if recorded.Dir != physicalPath(t, requested) {
		t.Errorf("the job ran in %q, want the configured working directory %q",
			recorded.Dir, physicalPath(t, requested))
	}
}

func TestAWorkingDirectoryThatDoesNotExistIsASpawnError(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("working_directory.toml")

	// EXERCISE
	outcome := environment.runOnce(configPath, "missing")

	// VERIFY
	if outcome.code == 0 {
		t.Fatalf("run-once succeeded, want a failure for a job that cannot start\nstdout:\n%s", outcome.stdout)
	}
	if !strings.Contains(outcome.stdout, string(job.StatusSpawnError)) {
		t.Errorf("run-once printed %q, want it to report %q", outcome.stdout, job.StatusSpawnError)
	}

	run := environment.latestRun("missing")
	if run.Status != job.StatusSpawnError {
		t.Errorf("the run was recorded as %q, want %q", run.Status, job.StatusSpawnError)
	}
	if run.ExitCode != nil {
		t.Errorf("the run recorded the exit code %d, want none because no process ran", *run.ExitCode)
	}
	if run.Error == "" {
		t.Error("the run was recorded without an explanation, want the failure to be described")
	}
	// The explanation the machine produces for this case names the command
	// rather than the directory that is missing, because the operating system
	// reports a failed chdir as a failed fork of the program. The status and
	// the absence of a process are what cronx promises; the wording is only
	// checked to be present, so that improving it does not break a test.
	if _, err := os.Stat(environment.path("missing.json")); err == nil {
		t.Error("the job ran, want no process to have been started at all")
	}
}

func TestAJobSeesOnlyTheEnvironmentItWasGiven(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("environment.toml")

	// EXERCISE: the scheduler itself runs with a variable of its own, which
	// must not reach the job.
	environment.runOK("run-once", "environment", "--config", configPath)
	recorded := environment.waitForReport(environment.path("environment.json"), settleBudget)

	// VERIFY: the environment of a job is the documented minimal one plus what
	// the configuration declares, and nothing else.
	want := map[string]string{
		"PATH":             "/usr/local/bin:/usr/bin:/bin",
		"CRONX_TEST_ALPHA": "first",
		"CRONX_TEST_BETA":  "with spaces and $dollars",
	}
	if !maps.Equal(recorded.Env, want) {
		t.Errorf("the job received the environment\n%v\nwant exactly\n%v", recorded.Env, want)
	}
	for _, name := range []string{"CRONX_TEST_LEAK", "HOME", "TZ"} {
		if value, present := recorded.Env[name]; present {
			t.Errorf("the job received %s=%q, want the environment of the scheduler not to be inherited",
				name, value)
		}
	}
}
