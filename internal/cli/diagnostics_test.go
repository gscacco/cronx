package cli_test

import (
	"strings"
	"testing"
)

// brokenProblems is everything examples/configs/broken.toml gets wrong on
// purpose, exactly as "validate" reports it: one entry per mistake in the file.
var brokenProblems = []string{
	"scheduler.max_parallel_jobs must be at least 1, got 0",
	`logging.level "verbose" is not one of debug, error, info, warn`,
	`job "backup": schedule is not valid: cron expression "0 3 * *" must have 5 fields, got 4`,
	`job "backup": command "backup" must be an absolute path`,
	`job "backup": retry must not be negative, got -1`,
	`job "cleanup": schedule is not valid: cron expression "@every-minute" is not a known` +
		` descriptor: use one of @yearly, @monthly, @weekly, @daily, @midnight, @hourly`,
	`job "cleanup": overlap "sometimes" is not one of skip, allow, queue`,
	`job "cleanup": timeout "30 minutes" is not a valid duration`,
	`job "report": schedule is required`,
}

func TestTheBrokenConfigurationReportsEveryProblem(t *testing.T) {
	// SETUP
	newTestHome(t)

	// EXERCISE
	output, err := runCLI(t, "validate", "--config", examplePath(brokenExample))

	// VERIFY
	if err == nil {
		t.Fatalf("validate succeeded, want the broken configuration to be rejected")
	}
	if output != "" {
		t.Errorf("validate output = %q, want nothing printed for a rejected file", output)
	}
	for _, problem := range brokenProblems {
		if !strings.Contains(err.Error(), problem) {
			t.Errorf("validate error = %q, want it to report %q", err.Error(), problem)
		}
	}
}
