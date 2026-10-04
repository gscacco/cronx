package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The tests execute the test binary itself as the program of a job, so that
// they never depend on a shell or on an external program. The variables below
// tell the helper what to do.
const (
	helperMarker  = "CRONX_TEST_HELPER"
	helperMode    = "CRONX_TEST_MODE"
	helperExit    = "CRONX_TEST_EXIT"
	helperMessage = "CRONX_TEST_MESSAGE"
)

// helperSleep is how long the "sleep" mode waits before returning. It is longer
// than any timeout the tests configure, so a run only ends on time when the
// timeout is enforced.
const helperSleep = 30 * time.Second

// printedByTheJob is what the helper writes to its standard output, so that the
// tests can find it in the log file of a run.
const printedByTheJob = "printed by the job"

// TestHelperProcess is not a real test. Without the marker variable it returns
// immediately, which is what happens when the suite runs normally.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperMarker) != "1" {
		return
	}
	switch os.Getenv(helperMode) {
	case "ok":
		// Nothing to do.
	case "print":
		fmt.Println(printedByTheJob)
	case "report":
		reportToStdout()
	case "fail":
		code, err := strconv.Atoi(os.Getenv(helperExit))
		if err != nil {
			os.Exit(2)
		}
		os.Exit(code)
	case "sleep":
		time.Sleep(helperSleep)
	}
	os.Exit(0)
}

// reportToStdout prints what the job received: its working directory, the value
// of a variable of its own and the names of every variable in its environment.
// The tests compare this with what the configuration asked for.
func reportToStdout() {
	directory, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "getwd:", err)
		os.Exit(2)
	}
	fmt.Println("cwd=" + directory)
	fmt.Println("message=" + os.Getenv(helperMessage))
	fmt.Println("env=" + strings.Join(environmentNames(), ","))
}

// environmentNames returns the names of the variables of the current process in
// ascending order.
func environmentNames() []string {
	names := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// testExecutable returns the absolute path of the test binary, which the tests
// use as the program of a job.
func testExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test executable: %v", err)
	}
	return executable
}

// readFixture returns the contents of a file in the testdata directory, which
// holds the configuration files the tests are driven by.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the fixture %s: %v", path, err)
	}
	return string(contents)
}

// readFile returns the contents of a file, failing the test when it cannot be
// read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(contents)
}
