package scheduler_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/logx"
	"gscacco.com/cronx/internal/runner"
	"gscacco.com/cronx/internal/scheduler"
	"gscacco.com/cronx/internal/store"
)

// The scheduler executes real programs. The test binary re-executes itself and
// behaves according to environment variables, so no shell is ever involved.
const (
	helperMarker  = "CRONX_TEST_HELPER"
	helperMode    = "CRONX_TEST_MODE"
	helperSleep   = "CRONX_TEST_SLEEP"
	helperExit    = "CRONX_TEST_EXIT"
	helperCounter = "CRONX_TEST_COUNTER"
	helperFailFor = "CRONX_TEST_FAIL_FOR"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperMarker) != "1" {
		return
	}

	switch os.Getenv(helperMode) {
	case "ok":
		// Nothing to do: exit successfully.
	case "fail":
		os.Exit(exitCodeFromEnv(helperExit))
	case "print":
		fmt.Println("job output")
	case "sleep":
		sleepFor(os.Getenv(helperSleep))
	case "fail-times":
		if bumpCounter(os.Getenv(helperCounter)) <= exitCodeFromEnv(helperFailFor) {
			os.Exit(1)
		}
	}

	os.Exit(0)
}

// sleepFor sleeps for the duration held in value.
func sleepFor(value string) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid duration:", err)
		os.Exit(2)
	}
	time.Sleep(duration)
}

// exitCodeFromEnv reads an integer from the environment, defaulting to zero.
func exitCodeFromEnv(name string) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return 0
	}
	return value
}

// bumpCounter increments the counter stored in path and returns the new value.
func bumpCounter(path string) int {
	current := 0
	if content, err := os.ReadFile(path); err == nil {
		current, _ = strconv.Atoi(string(content))
	}
	current++
	if err := os.WriteFile(path, []byte(strconv.Itoa(current)), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "counter:", err)
		os.Exit(2)
	}
	return current
}

// executable returns the absolute path of the test binary.
func executable(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test executable: %v", err)
	}
	return path
}

// helperJob builds a job that runs the test binary as the helper process in the
// given mode.
func helperJob(t *testing.T, name, mode string) job.Job {
	t.Helper()
	return job.Job{
		Name:     name,
		Schedule: everyMinute,
		Command:  executable(t),
		Args:     []string{"-test.run=TestHelperProcess"},
		Overlap:  job.DefaultOverlap,
		Env: map[string]string{
			helperMarker: "1",
			helperMode:   mode,
		},
	}
}

// everyMinute is the schedule used by tests that never depend on the clock.
const everyMinute = "* * * * *"

// buildScheduler wires a scheduler around an in-memory store and a shared run
// log in a temporary directory.
func buildScheduler(t *testing.T, jobs ...job.Job) (*scheduler.Scheduler, *store.Store, *logx.Log) {
	t.Helper()
	return buildSchedulerWithClock(t, clock.System{}, jobs...)
}

// buildSchedulerWithClock is buildScheduler with an explicit clock.
func buildSchedulerWithClock(t *testing.T, clk clock.Clock, jobs ...job.Job) (*scheduler.Scheduler, *store.Store, *logx.Log) {
	t.Helper()

	persistent, err := store.OpenMemory()
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	t.Cleanup(func() { _ = persistent.Close() })

	runs := logx.Open(filepath.Join(t.TempDir(), "logs", "runs.log"), clk)
	t.Cleanup(func() { _ = runs.Close() })

	logger, err := logx.NewLogger(io.Discard, "error")
	if err != nil {
		t.Fatalf("building the logger: %v", err)
	}

	configured := make(map[string]job.Job, len(jobs))
	for _, definition := range jobs {
		configured[definition.Name] = definition
	}

	built, err := scheduler.New(scheduler.Options{
		Config: config.Config{
			Scheduler: config.Scheduler{Timezone: "Local", MaxParallelJobs: 4},
			Logging:   config.Logging{Level: "error"},
			Jobs:      configured,
		},
		Store:  persistent,
		Logs:   runs,
		Runner: runner.New(clk),
		Clock:  clk,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("building the scheduler: %v", err)
	}
	return built, persistent, runs
}

// waitFor polls condition until it holds or the deadline passes.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
