// Package integration_test verifies cronx from the outside.
//
// Every test in this package drives the compiled cronx binary as a real
// process, with its real configuration parsing, its real scheduler, its real
// SQLite database and its real log files. Nothing is mocked and nothing is
// linked in-process: what the tests observe is what a user of cronx observes.
//
// The two programs the tests need are built once, in TestMain, and thrown away
// with the test run.
package integration_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The programs the tests drive.
var (
	// cronxBinary is the compiled scheduler, built from cmd/cronx.
	cronxBinary string
	// jobHelper is the compiled program a job runs, built from
	// testdata/job.
	jobHelper string
)

// TestMain builds the programs the tests drive and then runs the tests.
func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration tests:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

// run builds the programs in a directory of its own and runs the tests.
func run(m *testing.M) (int, error) {
	build, err := os.MkdirTemp("", "cronx-integration-")
	if err != nil {
		return 0, fmt.Errorf("creating the build directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(build) }()

	module, err := moduleRoot()
	if err != nil {
		return 0, err
	}

	cronxBinary = filepath.Join(build, "cronx")
	if err := buildPackage(module, build, cronxBinary, "./cmd/cronx"); err != nil {
		return 0, err
	}

	jobHelper = filepath.Join(build, "job")
	if err := buildPackage(module, build, jobHelper, "./test/integration/testdata/job"); err != nil {
		return 0, err
	}

	return m.Run(), nil
}

// moduleRoot returns the root of the module under test, which is the directory
// two levels above this package.
func moduleRoot() (string, error) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return "", fmt.Errorf("locating the module root: %w", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("locating the module root: %w", err)
	}
	return root, nil
}

// buildPackage compiles one package of the module into target. The package is
// named by an explicit path so that the job helper, which lives under testdata,
// is built even though the usual package patterns skip that directory.
func buildPackage(module, work, target, pkg string) error {
	command := exec.Command("go", "build", "-o", target, pkg)
	command.Dir = module
	command.Env = os.Environ()
	// "go" needs somewhere to keep its build cache. A development shell and
	// the check phase of the Nix package both provide one; a bare environment
	// does not, and the build would fail before it started.
	if os.Getenv("GOCACHE") == "" && os.Getenv("HOME") == "" {
		command.Env = append(command.Env, "GOCACHE="+filepath.Join(work, "build-cache"))
	}

	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("building %s: %w\n%s", pkg, err, output)
	}
	return nil
}
