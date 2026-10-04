package config_test

import (
	"path/filepath"
	"testing"

	"gscacco.com/cronx/internal/config"
)

func TestDefaultPath(t *testing.T) {
	// SETUP
	home := t.TempDir()
	t.Setenv("HOME", home)

	// EXERCISE
	got, err := config.DefaultPath()

	// VERIFY
	if err != nil {
		t.Fatalf("DefaultPath() returned an unexpected error: %v", err)
	}
	want := filepath.Join(home, ".cronx", "config.toml")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestResolvePathPrecedence(t *testing.T) {
	// SETUP
	home := t.TempDir()
	t.Setenv("HOME", home)
	defaultPath := filepath.Join(home, ".cronx", "config.toml")
	envPath := filepath.Join(home, "from-env.toml")
	flagPath := filepath.Join(home, "from-flag.toml")

	t.Run("the flag wins over the environment and the default", func(t *testing.T) {
		// SETUP
		t.Setenv(config.EnvVar, envPath)

		// EXERCISE
		got, err := config.ResolvePath(flagPath)

		// VERIFY
		if err != nil {
			t.Fatalf("ResolvePath() returned an unexpected error: %v", err)
		}
		if got != flagPath {
			t.Errorf("ResolvePath() = %q, want %q", got, flagPath)
		}
	})

	t.Run("the environment wins over the default", func(t *testing.T) {
		// SETUP
		t.Setenv(config.EnvVar, envPath)

		// EXERCISE
		got, err := config.ResolvePath("")

		// VERIFY
		if err != nil {
			t.Fatalf("ResolvePath() returned an unexpected error: %v", err)
		}
		if got != envPath {
			t.Errorf("ResolvePath() = %q, want %q", got, envPath)
		}
	})

	t.Run("the default is used when nothing is set", func(t *testing.T) {
		// SETUP
		t.Setenv(config.EnvVar, "")

		// EXERCISE
		got, err := config.ResolvePath("")

		// VERIFY
		if err != nil {
			t.Fatalf("ResolvePath() returned an unexpected error: %v", err)
		}
		if got != defaultPath {
			t.Errorf("ResolvePath() = %q, want %q", got, defaultPath)
		}
	})
}
