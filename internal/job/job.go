// Package job defines the cronx domain model: a job and its execution policy.
package job

import (
	"strings"
	"time"
)

// OverlapPolicy determines what happens when a job is triggered while a
// previous run of the same job is still running.
type OverlapPolicy string

const (
	// OverlapSkip does not start a new run; the trigger is recorded as skipped.
	OverlapSkip OverlapPolicy = "skip"
	// OverlapAllow starts a new run even if a previous one is still running.
	OverlapAllow OverlapPolicy = "allow"
	// OverlapQueue starts a new run as soon as the previous one finishes.
	OverlapQueue OverlapPolicy = "queue"
)

// DefaultOverlap is the overlap policy applied when a job does not set one.
const DefaultOverlap = OverlapSkip

// validOverlaps lists every accepted overlap policy.
var validOverlaps = []OverlapPolicy{OverlapSkip, OverlapAllow, OverlapQueue}

// IsValidOverlap reports whether p is one of the accepted overlap policies.
func IsValidOverlap(p OverlapPolicy) bool {
	for _, valid := range validOverlaps {
		if p == valid {
			return true
		}
	}
	return false
}

// OverlapList returns the accepted overlap policies as a comma-separated list,
// for use in error messages.
func OverlapList() string {
	values := make([]string, len(validOverlaps))
	for i, p := range validOverlaps {
		values[i] = string(p)
	}
	return strings.Join(values, ", ")
}

// Job is an executable program together with its execution policy.
type Job struct {
	// Name is the identifier of the job, as written in the configuration.
	Name string
	// Schedule is the raw cron expression. It is parsed by the scheduler.
	Schedule string
	// Command is the absolute path of the executable. cronx never resolves it
	// through PATH and never runs it through a shell.
	Command string
	// Args are passed to the executable verbatim, never through a shell.
	Args []string
	// Timeout bounds the execution time. Zero means no timeout.
	Timeout time.Duration
	// Retry is the number of additional attempts after the first attempt fails.
	Retry int
	// Overlap determines the behaviour when a run is still in progress.
	Overlap OverlapPolicy
	// WorkingDirectory is the process working directory. Empty means the
	// scheduler's working directory.
	WorkingDirectory string
	// Env holds extra environment variables for the process. The parent
	// environment is not inherited.
	Env map[string]string
}
