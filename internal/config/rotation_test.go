package config_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/config"
)

// rotationConfig describes a configuration that asks for the run log to be
// rotated.
const rotationConfig = validJob + `
[logging]
max_size = "10MB"
max_backups = 2
`

func TestMaxSizeIsReadInBytes(t *testing.T) {
	// SETUP
	data := []byte(rotationConfig)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Logging.MaxSize, int64(10*1024*1024); got != want {
		t.Errorf("Logging.MaxSize = %d, want %d", got, want)
	}
	if got, want := cfg.Logging.MaxBackups, 2; got != want {
		t.Errorf("Logging.MaxBackups = %d, want %d", got, want)
	}
}

func TestASizeIsReadInTheUnitItIsWrittenIn(t *testing.T) {
	// SETUP: the same size, written in the units a configuration may use.
	for _, format := range []string{"2048", "2048B", "2KB", "2kb"} {
		data := []byte(validJob + "\n[logging]\nmax_size = \"" + format + "\"\n")

		// EXERCISE
		cfg, err := config.Parse(data)

		// VERIFY
		if err != nil {
			t.Fatalf("Parse() with max_size = %q returned an unexpected error: %v", format, err)
		}
		if got, want := cfg.Logging.MaxSize, int64(2048); got != want {
			t.Errorf("Logging.MaxSize for %q = %d, want %d", format, got, want)
		}
	}
}

func TestRotationKeepsThreeBackupsUnlessAsked(t *testing.T) {
	// SETUP: a size without a number of backups.
	data := []byte(validJob + "\n[logging]\nmax_size = \"1MB\"\n")

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Logging.MaxBackups, config.DefaultLogBackups; got != want {
		t.Errorf("Logging.MaxBackups = %d, want the default %d", got, want)
	}
}

func TestTheRunLogIsNotRotatedUnlessASizeIsGiven(t *testing.T) {
	// SETUP: a configuration that says nothing about rotation.
	data := []byte(validJob)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Logging.MaxSize, int64(0); got != want {
		t.Errorf("Logging.MaxSize = %d, want %d: no log is rotated by default", got, want)
	}
	if got, want := cfg.Logging.MaxBackups, 0; got != want {
		t.Errorf("Logging.MaxBackups = %d, want %d: nothing is rotated, so nothing is kept", got, want)
	}
}

func TestParseRejectsRotationSettingsItCannotUse(t *testing.T) {
	tests := []struct {
		name    string
		logging string
		wantErr string
	}{
		{
			name:    "a size that is not a size",
			logging: "max_size = \"10 megabytes\"",
			wantErr: "logging.max_size",
		},
		{
			name:    "a size without a number",
			logging: "max_size = \"MB\"",
			wantErr: "logging.max_size",
		},
		{
			name:    "a size of zero",
			logging: "max_size = \"0B\"",
			wantErr: "logging.max_size",
		},
		{
			name:    "a negative size",
			logging: "max_size = \"-1KB\"",
			wantErr: "logging.max_size",
		},
		{
			name:    "backups without a size",
			logging: "max_backups = 2",
			wantErr: "logging.max_backups",
		},
		{
			name:    "no backup at all",
			logging: "max_size = \"1MB\"\nmax_backups = 0",
			wantErr: "logging.max_backups",
		},
		{
			name:    "a negative number of backups",
			logging: "max_size = \"1MB\"\nmax_backups = -1",
			wantErr: "logging.max_backups",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// SETUP
			data := []byte(validJob + "\n[logging]\n" + test.logging + "\n")

			// EXERCISE
			_, err := config.Parse(data)

			// VERIFY
			if err == nil {
				t.Fatalf("Parse() succeeded with %q, want an error", test.logging)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("Parse() error = %q, want it to name %q", err.Error(), test.wantErr)
			}
		})
	}
}
