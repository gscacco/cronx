package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/runner"
)

func TestRunNeverInvokesAShell(t *testing.T) {
	// SETUP
	output := helperOutFile(t)
	sideEffect := filepath.Join(t.TempDir(), "created-by-a-shell")
	shellArgument := "; touch " + sideEffect
	command := newHelperCommand(t, "record-args", output)
	command.Args = append(command.Args, shellArgument)

	// EXERCISE
	_, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if _, statErr := os.Stat(sideEffect); !os.IsNotExist(statErr) {
		t.Fatalf("the metacharacters in %q were interpreted: %v exists", shellArgument, sideEffect)
	}
	if got := readRecorded(t, output); got != shellArgument {
		t.Errorf("recorded argument = %q, want the literal argument %q", got, shellArgument)
	}
}

func TestRunPassesArgumentsVerbatim(t *testing.T) {
	// SETUP
	output := helperOutFile(t)
	arguments := []string{
		"a b c",
		"$(whoami)",
		"`id`",
		"*",
		"> /tmp/redirect",
		"--flag=value",
		"",
	}
	command := newHelperCommand(t, "record-args", output)
	command.Args = append(command.Args, arguments...)

	// EXERCISE
	_, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	recorded := strings.Split(readRecorded(t, output), "\n")
	if len(recorded) != len(arguments) {
		t.Fatalf("recorded %d arguments, want %d: %q", len(recorded), len(arguments), recorded)
	}
	for index, want := range arguments {
		if recorded[index] != want {
			t.Errorf("argument %d = %q, want %q", index, recorded[index], want)
		}
	}
}

func TestRunDoesNotInheritTheSchedulerEnvironment(t *testing.T) {
	// SETUP
	t.Setenv("CRONX_TEST_SECRET", "leaked")
	output := helperOutFile(t)
	command := newHelperCommand(t, "record-env", output)
	command.Env[helperName] = "CRONX_TEST_SECRET"

	// EXERCISE
	_, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if got := readRecorded(t, output); got != "" {
		t.Errorf("the process saw CRONX_TEST_SECRET=%q, want the scheduler environment not to be inherited", got)
	}
}

// jobVariable is the variable the helper process reads back for the test.
const jobVariable = "CRONX_TEST_VALUE"

func TestRunAppliesTheJobEnvironment(t *testing.T) {
	// SETUP
	output := helperOutFile(t)
	command := newHelperCommand(t, "record-env", output)
	command.Env[helperName] = jobVariable
	command.Env[jobVariable] = "explicit-value"

	// EXERCISE
	_, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if got := readRecorded(t, output); got != "explicit-value" {
		t.Errorf("the process saw %s=%q, want %q", jobVariable, got, "explicit-value")
	}
}

func TestRunProvidesTheDocumentedMinimalPath(t *testing.T) {
	// SETUP
	output := helperOutFile(t)
	command := newHelperCommand(t, "record-env", output)
	command.Env[helperName] = "PATH"

	// EXERCISE
	_, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v", err)
	}
	if got := readRecorded(t, output); got != runner.DefaultPath {
		t.Errorf("PATH = %q, want %q", got, runner.DefaultPath)
	}
}

func TestRunRejectsANonAbsoluteCommandPath(t *testing.T) {
	// SETUP
	command := runner.Command{Path: "true"}

	// EXERCISE
	_, err := runner.New(clock.System{}).Run(context.Background(), command)

	// VERIFY
	if err == nil {
		t.Fatalf("Run() succeeded, want an error: cronx must never resolve a command through PATH")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("Run() error = %q, want it to explain that the path must be absolute", err.Error())
	}
}
