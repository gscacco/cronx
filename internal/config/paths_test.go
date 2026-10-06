package config_test

import (
	"path/filepath"
	"testing"

	"gscacco.com/cronx/internal/config"
)

// configuredPaths is a configuration that sets both optional paths.
const configuredPaths = `
[logging]
path = "/var/log/cronx/runs.log"

[storage]
path = "/var/lib/cronx/state.db"

[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
`

func TestParseReadsTheConfiguredPaths(t *testing.T) {
	// SETUP
	data := []byte(configuredPaths)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got, want := cfg.Logging.Path, "/var/log/cronx/runs.log"; got != want {
		t.Errorf("Logging.Path = %q, want %q", got, want)
	}
	if got, want := cfg.Storage.Path, "/var/lib/cronx/state.db"; got != want {
		t.Errorf("Storage.Path = %q, want %q", got, want)
	}
}

func TestThePathsAreEmptyWhenTheyAreNotConfigured(t *testing.T) {
	// SETUP
	data := []byte(minimalConfig)

	// EXERCISE
	cfg, err := config.Parse(data)

	// VERIFY: an unset path is left to the default resolution, not filled in
	// here, because the default depends on the home directory.
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}
	if got := cfg.Logging.Path; got != "" {
		t.Errorf("Logging.Path = %q, want it empty when the configuration does not set it", got)
	}
	if got := cfg.Storage.Path; got != "" {
		t.Errorf("Storage.Path = %q, want it empty when the configuration does not set it", got)
	}
}

func TestThePathsDefaultToTheHomeDirectory(t *testing.T) {
	// SETUP
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("the runs log", func(t *testing.T) {
		// EXERCISE
		got, err := config.LogPath("")

		// VERIFY
		if err != nil {
			t.Fatalf("LogPath() returned an unexpected error: %v", err)
		}
		if want := filepath.Join(home, ".cronx", "logs", "runs.log"); got != want {
			t.Errorf("LogPath() = %q, want %q", got, want)
		}
	})

	t.Run("the state database", func(t *testing.T) {
		// EXERCISE
		got, err := config.StatePath("")

		// VERIFY
		if err != nil {
			t.Fatalf("StatePath() returned an unexpected error: %v", err)
		}
		if want := filepath.Join(home, ".cronx", "state.db"); got != want {
			t.Errorf("StatePath() = %q, want %q", got, want)
		}
	})
}

func TestAConfiguredPathWinsOverTheDefault(t *testing.T) {
	// SETUP
	t.Setenv("HOME", t.TempDir())
	configuredLog := "/srv/cronx/cronx-runs.log"
	configuredState := "/srv/cronx/cronx.db"

	// EXERCISE
	logPath, logErr := config.LogPath(configuredLog)
	statePath, stateErr := config.StatePath(configuredState)

	// VERIFY
	if logErr != nil {
		t.Fatalf("LogPath() returned an unexpected error: %v", logErr)
	}
	if logPath != configuredLog {
		t.Errorf("LogPath() = %q, want the configured %q", logPath, configuredLog)
	}
	if stateErr != nil {
		t.Fatalf("StatePath() returned an unexpected error: %v", stateErr)
	}
	if statePath != configuredState {
		t.Errorf("StatePath() = %q, want the configured %q", statePath, configuredState)
	}
}
