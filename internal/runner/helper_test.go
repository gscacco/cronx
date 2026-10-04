package runner_test

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gscacco.com/cronx/internal/runner"
)

// The test binary doubles as the program the runner executes. It behaves
// according to environment variables and never uses a shell, exactly as cronx
// must behave at runtime.
const (
	helperMarker = "CRONX_TEST_HELPER"
	helperMode   = "CRONX_TEST_MODE"
	helperSleep  = "CRONX_TEST_SLEEP"
	helperExit   = "CRONX_TEST_EXIT"
	helperName   = "CRONX_TEST_NAME"
	helperOut    = "CRONX_TEST_OUT"
)

// TestHelperProcess is not a real test: it is the entry point executed when the
// runner launches the test binary again. Without the marker variable it returns
// immediately.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperMarker) != "1" {
		return
	}

	switch os.Getenv(helperMode) {
	case "record-args":
		record(os.Getenv(helperOut), strings.Join(argsAfterSeparator(os.Args), "\n"))
	case "record-env":
		record(os.Getenv(helperOut), os.Getenv(os.Getenv(helperName)))
	case "record-pgid":
		group, err := recordProcessGroup()
		if err != nil {
			fmt.Fprintln(os.Stderr, "getpgid:", err)
			os.Exit(2)
		}
		record(os.Getenv(helperOut), group)
	case "record-cwd":
		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "getwd:", err)
			os.Exit(2)
		}
		record(os.Getenv(helperOut), dir)
	case "print":
		fmt.Println("stdout-line")
		fmt.Fprintln(os.Stderr, "stderr-line")
	case "exit":
		code, err := strconv.Atoi(os.Getenv(helperExit))
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid exit code:", err)
			os.Exit(2)
		}
		os.Exit(code)
	case "sleep":
		sleepFor(os.Getenv(helperSleep))
	case "trap-term":
		caught := make(chan os.Signal, 1)
		signal.Notify(caught, syscall.SIGTERM)
		<-caught
		fmt.Println("terminated")
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		sleepFor(os.Getenv(helperSleep))
	default:
		fmt.Fprintln(os.Stderr, "unknown helper mode:", os.Getenv(helperMode))
		os.Exit(2)
	}

	os.Exit(0)
}

// sleepFor sleeps for the duration held in value, failing the helper if the
// duration cannot be parsed.
func sleepFor(value string) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid duration:", err)
		os.Exit(2)
	}
	time.Sleep(duration)
}

// record writes content to path, failing the helper on error.
func record(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		os.Exit(2)
	}
}

// argsAfterSeparator returns the arguments that follow the first "--".
func argsAfterSeparator(args []string) []string {
	for index, arg := range args {
		if arg == "--" {
			return args[index+1:]
		}
	}
	return nil
}

// helperExecutable returns the absolute path of the test binary.
func helperExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test executable: %v", err)
	}
	return executable
}

// newHelperCommand builds a command that runs the test binary in the given
// helper mode, recording its output in the file at path (when path is not
// empty).
func newHelperCommand(t *testing.T, mode, path string) runner.Command {
	t.Helper()
	env := map[string]string{
		helperMarker: "1",
		helperMode:   mode,
	}
	if path != "" {
		env[helperOut] = path
	}
	return runner.Command{
		Path: helperExecutable(t),
		Args: []string{"-test.run=TestHelperProcess", "--"},
		Env:  env,
	}
}

// helperOutFile returns the path of a fresh file the helper can write to.
func helperOutFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "out.txt")
}

// readRecorded returns what the helper process recorded in path.
func readRecorded(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the helper output %s: %v", path, err)
	}
	return string(content)
}
