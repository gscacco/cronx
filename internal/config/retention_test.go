package config_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/config"
)

func TestMaxRunsIsRead(t *testing.T) {
	// SETUP
	data := []byte(validJob + "\n[storage]\nmax_runs = 500\n")

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Storage.MaxRuns, 500; got != want {
		t.Errorf("Storage.MaxRuns = %d, want %d", got, want)
	}
}

func TestTheHistoryIsKeptInFullUnlessAsked(t *testing.T) {
	// SETUP: a configuration that says nothing about the history.
	data := []byte(validJob)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Storage.MaxRuns, 0; got != want {
		t.Errorf("Storage.MaxRuns = %d, want %d: nothing is pruned by default", got, want)
	}
}

func TestParseRejectsAMaxRunsItCannotUse(t *testing.T) {
	tests := []struct {
		name    string
		storage string
		wantErr string
	}{
		{
			name:    "a history of no run",
			storage: "max_runs = 0",
			wantErr: "storage.max_runs",
		},
		{
			name:    "a negative history",
			storage: "max_runs = -10",
			wantErr: "storage.max_runs",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// SETUP
			data := []byte(validJob + "\n[storage]\n" + test.storage + "\n")

			// EXERCISE
			_, err := config.Parse(data)

			// VERIFY
			if err == nil {
				t.Fatalf("Parse() succeeded with %q, want an error", test.storage)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("Parse() error = %q, want it to name %q", err.Error(), test.wantErr)
			}
		})
	}
}
