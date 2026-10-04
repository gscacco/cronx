// Package config loads and validates the cronx configuration.
//
// The configuration is the desired state of the scheduler. It is written in
// TOML and is the single source of truth for the jobs cronx should run.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"gscacco.com/cronx/internal/job"
)

// EnvVar is the environment variable that overrides the configuration path.
const EnvVar = "CRONX_CONFIG"

// Defaults applied when the configuration does not set a value.
const (
	DefaultTimezone        = "Local"
	DefaultMaxParallelJobs = 1
	DefaultLoggingLevel    = "info"
)

// Directory and file name of the default configuration, relative to the user's
// home directory.
const (
	configDirName  = ".cronx"
	configFileName = "config.toml"
)

// jobNamePattern bounds the characters allowed in a job name.
var jobNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// validLoggingLevels is the set of accepted logging levels.
var validLoggingLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Scheduler holds the global scheduler settings.
type Scheduler struct {
	// Timezone is an IANA timezone name or "Local". Resolution to a location
	// happens when the scheduler starts.
	Timezone string
	// MaxParallelJobs is the maximum number of jobs running at the same time.
	MaxParallelJobs int
}

// Logging holds the logging settings.
type Logging struct {
	// Level is one of debug, info, warn, error.
	Level string
}

// Config is the desired configuration of the scheduler.
type Config struct {
	Scheduler Scheduler
	Logging   Logging
	// Jobs are the configured jobs, keyed by name.
	Jobs map[string]job.Job
}

// DefaultPath returns the default configuration path: ~/.cronx/config.toml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining the user home directory: %w", err)
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

// ResolvePath returns the configuration path to use. An explicit flag value
// wins over the CRONX_CONFIG environment variable, which wins over the default.
func ResolvePath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if fromEnv := os.Getenv(EnvVar); fromEnv != "" {
		return fromEnv, nil
	}
	return DefaultPath()
}

// Load reads, parses and validates the configuration file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading configuration file %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses and validates a configuration held in memory.
func Parse(data []byte) (*Config, error) {
	var raw rawConfig
	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		return nil, fmt.Errorf("parsing configuration: %w", err)
	}

	cfg := &Config{
		Scheduler: Scheduler{
			Timezone:        DefaultTimezone,
			MaxParallelJobs: DefaultMaxParallelJobs,
		},
		Logging: Logging{Level: DefaultLoggingLevel},
		Jobs:    make(map[string]job.Job, len(raw.Jobs)),
	}

	var problems []error

	for _, key := range md.Undecoded() {
		problems = append(problems, fmt.Errorf("unknown configuration key %q", key.String()))
	}

	if raw.Scheduler.Timezone != "" {
		cfg.Scheduler.Timezone = raw.Scheduler.Timezone
	}
	if raw.Scheduler.MaxParallelJobs != nil {
		if *raw.Scheduler.MaxParallelJobs < 1 {
			problems = append(problems, fmt.Errorf(
				"scheduler.max_parallel_jobs must be at least 1, got %d",
				*raw.Scheduler.MaxParallelJobs))
		} else {
			cfg.Scheduler.MaxParallelJobs = *raw.Scheduler.MaxParallelJobs
		}
	}

	if raw.Logging.Level != "" {
		if !validLoggingLevels[raw.Logging.Level] {
			problems = append(problems, fmt.Errorf(
				"logging.level %q is not one of %s",
				raw.Logging.Level, strings.Join(sortedKeys(validLoggingLevels), ", ")))
		} else {
			cfg.Logging.Level = raw.Logging.Level
		}
	}

	// Iterate over sorted names so that error output is deterministic.
	for _, name := range sortedKeys(raw.Jobs) {
		built, jobProblems := buildJob(name, raw.Jobs[name])
		problems = append(problems, jobProblems...)
		if len(jobProblems) == 0 {
			cfg.Jobs[name] = built
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid configuration: %w", errors.Join(problems...))
	}
	return cfg, nil
}
