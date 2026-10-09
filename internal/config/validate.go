package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/schedule"
)

// buildJob converts a raw TOML job into the domain model, collecting every
// validation problem it finds.
func buildJob(name string, raw rawJob) (job.Job, []error) {
	built := job.Job{
		Name:             name,
		Schedule:         raw.Schedule,
		Command:          raw.Command,
		Args:             raw.Args,
		Retry:            raw.Retry,
		Overlap:          job.DefaultOverlap,
		WorkingDirectory: raw.WorkingDirectory,
		Env:              raw.Env,
	}

	var problems []error

	if !jobNamePattern.MatchString(name) {
		problems = append(problems, fmt.Errorf(
			"job name %q is invalid: it must match %s", name, jobNamePattern.String()))
	}
	if strings.TrimSpace(raw.Schedule) == "" {
		problems = append(problems, fmt.Errorf("job %q: schedule is required", name))
	} else if _, err := schedule.Parse(raw.Schedule); err != nil {
		problems = append(problems, fmt.Errorf("job %q: schedule is not valid: %w", name, err))
	}
	if strings.TrimSpace(raw.Command) == "" {
		problems = append(problems, fmt.Errorf("job %q: command is required", name))
	} else if !filepath.IsAbs(raw.Command) {
		problems = append(problems, fmt.Errorf(
			"job %q: command %q must be an absolute path", name, raw.Command))
	}
	if raw.Retry < 0 {
		problems = append(problems, fmt.Errorf(
			"job %q: retry must not be negative, got %d", name, raw.Retry))
	}
	if raw.Overlap != "" {
		policy := job.OverlapPolicy(raw.Overlap)
		if !job.IsValidOverlap(policy) {
			problems = append(problems, fmt.Errorf(
				"job %q: overlap %q is not one of %s", name, raw.Overlap, job.OverlapList()))
		} else {
			built.Overlap = policy
		}
	}
	if raw.Timeout != "" {
		timeout, err := time.ParseDuration(raw.Timeout)
		switch {
		case err != nil:
			problems = append(problems, fmt.Errorf(
				"job %q: timeout %q is not a valid duration", name, raw.Timeout))
		case timeout < 0:
			problems = append(problems, fmt.Errorf(
				"job %q: timeout must not be negative, got %s", name, raw.Timeout))
		default:
			built.Timeout = timeout
		}
	}
	if raw.GracePeriod != "" {
		grace, err := time.ParseDuration(raw.GracePeriod)
		switch {
		case err != nil:
			problems = append(problems, fmt.Errorf(
				"job %q: grace_period %q is not a valid duration", name, raw.GracePeriod))
		case grace <= 0:
			problems = append(problems, fmt.Errorf(
				"job %q: grace_period must be greater than 0, got %s", name, raw.GracePeriod))
		default:
			built.GracePeriod = grace
		}
	}

	return built, problems
}

// rawConfig mirrors the TOML document. Pointer fields distinguish an absent key
// from a zero value.
type rawConfig struct {
	Scheduler rawScheduler      `toml:"scheduler"`
	Logging   rawLogging        `toml:"logging"`
	Storage   rawStorage        `toml:"storage"`
	Jobs      map[string]rawJob `toml:"jobs"`
}

type rawScheduler struct {
	Timezone        string `toml:"timezone"`
	MaxParallelJobs *int   `toml:"max_parallel_jobs"`
}

type rawLogging struct {
	Level      string `toml:"level"`
	Path       string `toml:"path"`
	MaxSize    string `toml:"max_size"`
	MaxBackups *int   `toml:"max_backups"`
}

type rawStorage struct {
	Path string `toml:"path"`
}

type rawJob struct {
	Schedule         string            `toml:"schedule"`
	Command          string            `toml:"command"`
	Args             []string          `toml:"args"`
	Timeout          string            `toml:"timeout"`
	GracePeriod      string            `toml:"grace_period"`
	Retry            int               `toml:"retry"`
	Overlap          string            `toml:"overlap"`
	WorkingDirectory string            `toml:"working_directory"`
	Env              map[string]string `toml:"env"`
}

// sortedKeys returns the keys of m in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
