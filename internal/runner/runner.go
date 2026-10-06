// Package runner executes jobs securely.
//
// A job is always started as a program identified by an absolute path, with a
// list of arguments passed verbatim. cronx never invokes a shell and never
// resolves a command through PATH, so shell metacharacters given as arguments
// stay literal arguments.
//
// The job runs in its own process group: when it must be stopped, the whole
// group receives SIGTERM first and is killed with SIGKILL only if it is still
// running after a grace period.
package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"gscacco.com/cronx/internal/clock"
)

const (
	// DefaultGracePeriod is how long a job is given to exit after SIGTERM
	// before it is killed with SIGKILL.
	DefaultGracePeriod = 10 * time.Second

	// exitedBySignal is the exit code reported for a process terminated by a
	// signal.
	exitedBySignal = -1
)

// Command describes the program to execute.
type Command struct {
	// Path is the absolute path of the executable.
	Path string
	// Args are passed to the executable verbatim.
	Args []string
	// Dir is the working directory. Empty means the current directory.
	Dir string
	// Env holds extra environment variables. The scheduler's own environment
	// is never inherited.
	Env map[string]string
	// Timeout bounds the execution. Zero means no timeout.
	Timeout time.Duration
	// GracePeriod is how long to wait between SIGTERM and SIGKILL. Zero means
	// DefaultGracePeriod.
	GracePeriod time.Duration
	// Stdout and Stderr receive the output of the process. When nil the output
	// is discarded. Stdin is never connected.
	Stdout io.Writer
	Stderr io.Writer
	// OnStart, when set, is called with the identifier of the process as soon
	// as it has started, so that its output can be attributed to it.
	OnStart func(pid int)
}

// Result describes the outcome of an execution.
type Result struct {
	// ExitCode is the exit status of the process, or -1 when it was terminated
	// by a signal.
	ExitCode int
	// StartedAt is the instant at which the execution was attempted.
	StartedAt time.Time
	// FinishedAt is the instant at which the outcome was observed.
	FinishedAt time.Time
	// Duration is FinishedAt minus StartedAt.
	Duration time.Duration
	// TimedOut reports whether the process was stopped because Timeout expired.
	TimedOut bool
}

// Runner starts and supervises jobs.
type Runner struct {
	clock clock.Clock
}

// New returns a Runner that reads timestamps from the given clock.
func New(clk clock.Clock) *Runner {
	return &Runner{clock: clk}
}

// Run executes the command and returns its outcome.
//
// A non-nil error means the process could not be started at all; a process that
// starts and then fails, including one stopped because of a timeout, is
// reported through the returned Result.
func (r *Runner) Run(ctx context.Context, command Command) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	result := Result{StartedAt: r.clock.Now()}

	path, err := absolutePath(command.Path)
	if err != nil {
		return r.finish(result), err
	}
	if command.Timeout < 0 {
		return r.finish(result), fmt.Errorf("job timeout must not be negative, got %s", command.Timeout)
	}

	process := exec.Command(path, command.Args...)
	process.Dir = command.Dir
	process.Env = environment(command.Env)
	process.Stdout = command.Stdout
	process.Stderr = command.Stderr
	setProcessGroup(process)

	if startErr := process.Start(); startErr != nil {
		return r.finish(result), fmt.Errorf("starting %s: %w", path, startErr)
	}

	if command.OnStart != nil {
		command.OnStart(process.Process.Pid)
	}

	waitErr := r.wait(ctx, process, command, &result)
	result.ExitCode = exitCode(process, waitErr)

	return r.finish(result), nil
}

// finish records the end of the execution.
func (r *Runner) finish(result Result) Result {
	result.FinishedAt = r.clock.Now()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	return result
}

// absolutePath rejects anything that is not an absolute path. cronx never
// resolves a command through PATH.
func absolutePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("job command is empty")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("job command %q must be an absolute path", path)
	}
	return path, nil
}
