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
	// DefaultLogBackups is how many rotated run logs are kept when a maximum
	// size is configured without a number of backups.
	DefaultLogBackups = 3
)

// Directory and file names, relative to the user's home directory: the default
// configuration, the default state database and the default log.
const (
	configDirName     = ".cronx"
	configFileName    = "config.toml"
	logsDirectoryName = "logs"
	runsLogFileName   = "runs.log"
	stateFileName     = "state.db"
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
	// Timezone is an IANA timezone name or "Local": the zone whose clock the
	// scheduler computes activation times on. It is resolved to a location by
	// Location.
	Timezone string
	// MaxParallelJobs is the maximum number of jobs running at the same time.
	MaxParallelJobs int
}

// Logging holds the logging settings.
type Logging struct {
	// Level is one of debug, info, warn, error.
	Level string
	// Path is where the output of every job is written: a single file shared
	// by all runs. Empty means the default under the home directory, resolved
	// by LogPath.
	Path string
	// MaxSize is how many bytes the run log may reach before it is rotated.
	// Zero means it is never rotated and grows without bound.
	MaxSize int64
	// MaxBackups is how many rotated run logs are kept, the newest first, once
	// rotation is on. It is zero only when rotation is off.
	MaxBackups int
}

// Storage holds where cronx keeps what it writes.
type Storage struct {
	// Path is the status database: the execution history and the runtime
	// state. Empty means the default under the home directory, resolved by
	// StatePath.
	Path string
}

// Config is the desired configuration of the scheduler.
type Config struct {
	Scheduler Scheduler
	Logging   Logging
	Storage   Storage
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

// LogPath returns where the output of every job is written. The configured path
// wins; otherwise the log is ~/.cronx/logs/runs.log.
func LogPath(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining the user home directory: %w", err)
	}
	return filepath.Join(home, configDirName, logsDirectoryName, runsLogFileName), nil
}

// StatePath returns where the status database lives. The configured path wins;
// otherwise the database is ~/.cronx/state.db.
func StatePath(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining the user home directory: %w", err)
	}
	return filepath.Join(home, configDirName, stateFileName), nil
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
		Logging: Logging{Level: DefaultLoggingLevel, Path: raw.Logging.Path},
		Storage: Storage{Path: raw.Storage.Path},
		Jobs:    make(map[string]job.Job, len(raw.Jobs)),
	}

	var problems []error

	for _, key := range md.Undecoded() {
		problems = append(problems, fmt.Errorf("unknown configuration key %q", key.String()))
	}

	if raw.Scheduler.Timezone != "" {
		cfg.Scheduler.Timezone = raw.Scheduler.Timezone
	}
	if _, err := cfg.Scheduler.Location(); err != nil {
		problems = append(problems, err)
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

	if raw.Logging.MaxSize != "" {
		size, err := parseSize(raw.Logging.MaxSize)
		if err != nil {
			problems = append(problems, fmt.Errorf("logging.max_size %q is not a size: %w",
				raw.Logging.MaxSize, err))
		} else {
			cfg.Logging.MaxSize = size
			cfg.Logging.MaxBackups = DefaultLogBackups
		}
	}
	if raw.Logging.MaxBackups != nil {
		switch {
		case raw.Logging.MaxSize == "":
			problems = append(problems, fmt.Errorf(
				"logging.max_backups needs logging.max_size: without a size no log is rotated"))
		case *raw.Logging.MaxBackups < 1:
			problems = append(problems, fmt.Errorf(
				"logging.max_backups must be at least 1, got %d", *raw.Logging.MaxBackups))
		default:
			cfg.Logging.MaxBackups = *raw.Logging.MaxBackups
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
