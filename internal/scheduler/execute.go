package scheduler

import (
	"context"
	"fmt"
	"io"
	"time"

	"gscacco.com/cronx/internal/job"
	"gscacco.com/cronx/internal/runner"
	"gscacco.com/cronx/internal/store"
)

// Execute runs a job once, applying the overlap and retry policies, and records
// the outcome. The trigger instant is the time the run was scheduled for.
func (s *Scheduler) Execute(ctx context.Context, name string, trigger time.Time) (job.Status, error) {
	definition, known := s.config.Jobs[name]
	if !known {
		return "", fmt.Errorf("job %q is not configured", name)
	}

	release, proceed := s.acquire(definition)
	if !proceed {
		reason := fmt.Sprintf("a previous run is still in progress and the overlap policy is %q", definition.Overlap)
		if err := s.store.RecordSkipped(ctx, name, s.clock.Now(), reason); err != nil {
			return "", err
		}
		s.logger.Info("skipped a run", "job", name, "reason", reason)
		return job.StatusSkipped, nil
	}
	defer release()

	return s.attempt(ctx, definition, trigger), nil
}

// acquire applies the overlap policy of a job. It returns whether the run may
// proceed, together with the function that releases the job.
func (s *Scheduler) acquire(definition job.Job) (func(), bool) {
	lock, known := s.locks[definition.Name]
	if !known {
		return func() {}, true
	}

	switch definition.Overlap {
	case job.OverlapSkip:
		if !lock.TryLock() {
			return nil, false
		}
		return lock.Unlock, true
	case job.OverlapQueue:
		lock.Lock()
		return lock.Unlock, true
	default:
		// OverlapAllow and anything unexpected run straight away.
		return func() {}, true
	}
}

// attempt runs a job, retrying while it does not succeed.
func (s *Scheduler) attempt(ctx context.Context, definition job.Job, trigger time.Time) job.Status {
	status := job.StatusFailed

	for number := 1; number <= definition.Retry+1; number++ {
		status = s.runAttempt(ctx, definition, number, trigger)
		if status == job.StatusSucceeded {
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	return status
}

// runAttempt performs one execution and records it.
func (s *Scheduler) runAttempt(ctx context.Context, definition job.Job, number int, trigger time.Time) job.Status {
	name := definition.Name
	startedAt := s.clock.Now()

	id, err := s.store.StartRun(ctx, name, number, startedAt)
	if err != nil {
		s.logger.Error("recording the start of a run failed", "job", name, "error", err)
		return job.StatusFailed
	}

	// Every run appends to the same log. The writer is opened before the
	// process starts, so that the run is recorded even when it prints
	// nothing; a failure to open it is reported but does not stop the run.
	logPath := s.logs.Path()
	var (
		output  io.Writer
		onStart func(pid int)
	)
	writer, logErr := s.logs.Writer(name, id)
	if logErr != nil {
		s.logger.Error("opening the log of a run failed", "job", name, "error", logErr)
	} else {
		output = writer
		onStart = writer.SetPID
		defer func() { _ = writer.Close() }()
	}

	result, runErr := s.runner.Run(ctx, runner.Command{
		Path:    definition.Command,
		Args:    definition.Args,
		Dir:     definition.WorkingDirectory,
		Env:     definition.Env,
		Timeout: definition.Timeout,
		Stdout:  output,
		Stderr:  output,
		OnStart: onStart,
	})

	status := statusFor(result, runErr)
	finishedAt := result.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = s.clock.Now()
	}

	finish := store.Finish{
		Status:     status,
		FinishedAt: finishedAt,
		Duration:   result.Duration,
		LogPath:    logPath,
		Error:      explain(status, result, runErr),
	}
	if runErr == nil {
		exitCode := result.ExitCode
		finish.ExitCode = &exitCode
	}

	if err := s.store.FinishRun(ctx, id, finish); err != nil {
		s.logger.Error("recording the outcome of a run failed", "job", name, "run", id, "error", err)
		return status
	}

	s.logger.Info("job finished",
		"job", name, "run", id, "attempt", number, "status", string(status))
	return status
}

// statusFor maps the outcome of an execution onto a run status.
func statusFor(result runner.Result, err error) job.Status {
	switch {
	case err != nil:
		return job.StatusSpawnError
	case result.TimedOut:
		return job.StatusTimedOut
	case result.ExitCode == 0:
		return job.StatusSucceeded
	default:
		return job.StatusFailed
	}
}

// explain describes a non-successful outcome for the execution history.
func explain(status job.Status, result runner.Result, err error) string {
	if err != nil {
		return err.Error()
	}
	switch status {
	case job.StatusTimedOut:
		return fmt.Sprintf("the job was stopped after exceeding its timeout (exit code %d)", result.ExitCode)
	case job.StatusFailed:
		return fmt.Sprintf("the job exited with status %d", result.ExitCode)
	default:
		return ""
	}
}
