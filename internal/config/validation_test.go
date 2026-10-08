package config_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/config"
)

// validJob is the minimal valid job definition reused by the test cases below.
const validJob = `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
`

func TestParseRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string // substring expected in the error ("" means: any error)
	}{
		{
			name: "missing schedule",
			config: `
[jobs.backup]
command = "/usr/local/bin/backup"
`,
			wantErr: "schedule",
		},
		{
			name: "schedule with the wrong number of fields",
			config: `
[jobs.backup]
schedule = "0 3 * *"
command = "/usr/local/bin/backup"
`,
			wantErr: "schedule",
		},
		{
			name: "schedule with a value out of range",
			config: `
[jobs.backup]
schedule = "0 25 * * *"
command = "/usr/local/bin/backup"
`,
			wantErr: "schedule",
		},
		{
			name: "missing command",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
`,
			wantErr: "command",
		},
		{
			name: "command is not an absolute path",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
command = "backup"
`,
			wantErr: "absolute",
		},
		{
			name: "invalid overlap policy",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
overlap = "sometimes"
`,
			wantErr: "overlap",
		},
		{
			name: "negative retry",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
retry = -1
`,
			wantErr: "retry",
		},
		{
			name: "invalid timeout",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
timeout = "not-a-duration"
`,
			wantErr: "timeout",
		},
		{
			name: "invalid grace period",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
grace_period = "two seconds"
`,
			wantErr: "grace_period",
		},
		{
			name: "zero grace period",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
grace_period = "0s"
`,
			wantErr: "grace_period must be greater than 0",
		},
		{
			name: "negative grace period",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
grace_period = "-1s"
`,
			wantErr: "grace_period must be greater than 0",
		},
		{
			name: "invalid job name",
			config: `
[jobs."bad name"]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
`,
			wantErr: "name",
		},
		{
			name: "unknown job key",
			config: `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
scheduel = "typo"
`,
			wantErr: "unknown",
		},
		{
			name: "unknown top-level key",
			config: validJob + `
[schedulir]
timezone = "UTC"
`,
			wantErr: "unknown",
		},
		{
			name: "invalid logging level",
			config: validJob + `
[logging]
level = "loud"
`,
			wantErr: "level",
		},
		{
			name: "zero max_parallel_jobs",
			config: validJob + `
[scheduler]
max_parallel_jobs = 0
`,
			wantErr: "max_parallel_jobs",
		},
		{
			name: "negative max_parallel_jobs",
			config: validJob + `
[scheduler]
max_parallel_jobs = -1
`,
			wantErr: "max_parallel_jobs",
		},
		{
			name:    "malformed TOML",
			config:  `this is not = = valid toml`,
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SETUP
			data := []byte(tt.config)

			// EXERCISE
			_, err := config.Parse(data)

			// VERIFY
			if err == nil {
				t.Fatalf("Parse() succeeded, want an error containing %q", tt.wantErr)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Parse() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseAggregatesEveryError(t *testing.T) {
	// SETUP
	data := []byte(`
[jobs.backup]
overlap = "sometimes"
`)

	// EXERCISE
	_, err := config.Parse(data)

	// VERIFY
	if err == nil {
		t.Fatalf("Parse() succeeded, want an error")
	}
	for _, want := range []string{"schedule", "command", "overlap"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Parse() error = %q, want it to contain %q", err.Error(), want)
		}
	}
}
