package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gscacco.com/cronx/internal/job"
)

// timeFormat is how timestamps are stored: RFC 3339 in UTC.
const timeFormat = time.RFC3339Nano

// Run is one execution of a job.
type Run struct {
	// ID identifies the run within the database.
	ID int64
	// Job is the name of the job that ran.
	Job string
	// Attempt is the attempt number: 1 for the first try, then 2, 3... Zero
	// means the run never attempted an execution, for example a skipped run.
	Attempt int
	// Status is the outcome of the run.
	Status job.Status
	// StartedAt is when the run was attempted.
	StartedAt time.Time
	// FinishedAt is when the outcome was observed. It is the zero time while
	// the run is still in progress.
	FinishedAt time.Time
	// ExitCode is the exit status of the process. It is nil when no process ran.
	ExitCode *int
	// Duration is how long the run took.
	Duration time.Duration
	// Error describes a failure, when there was one.
	Error string
	// LogPath is where the output of the run was written.
	LogPath string
}

// Finish describes how a run ended.
type Finish struct {
	Status     job.Status
	FinishedAt time.Time
	Duration   time.Duration
	ExitCode   *int
	Error      string
	LogPath    string
}

// runColumns is the column list used by every query that reads runs.
const runColumns = "id, job, attempt, status, started_at, finished_at, exit_code, duration_ms, error, log_path"

// skippedAttempt marks a run that never attempted an execution.
const skippedAttempt = 0

// StartRun records that a job is about to run and returns the identifier of the
// new run.
func (s *Store) StartRun(ctx context.Context, jobName string, attempt int, startedAt time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (job, attempt, status, started_at) VALUES (?, ?, ?, ?)`,
		jobName, attempt, string(job.StatusRunning), formatTime(startedAt))
	if err != nil {
		return 0, fmt.Errorf("recording the start of job %q: %w", jobName, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading the identifier of the new run of job %q: %w", jobName, err)
	}
	return id, nil
}

// FinishRun records how a run ended and refreshes the cached state of its job.
func (s *Store) FinishRun(ctx context.Context, id int64, finish Finish) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs
		   SET status = ?, finished_at = ?, exit_code = ?, duration_ms = ?, error = ?, log_path = ?
		 WHERE id = ?`,
		string(finish.Status),
		formatTime(finish.FinishedAt),
		exitCodeValue(finish.ExitCode),
		finish.Duration.Milliseconds(),
		finish.Error,
		finish.LogPath,
		id)
	if err != nil {
		return fmt.Errorf("recording the outcome of run %d: %w", id, err)
	}
	return s.updateJobState(ctx, id, finish)
}

// RecordSkipped records a trigger that deliberately did not lead to an
// execution, such as one ignored by the overlap policy.
func (s *Store) RecordSkipped(ctx context.Context, jobName string, at time.Time, reason string) error {
	stamp := formatTime(at)
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (job, attempt, status, started_at, finished_at, error) VALUES (?, ?, ?, ?, ?, ?)`,
		jobName, skippedAttempt, string(job.StatusSkipped), stamp, stamp, reason)
	if err != nil {
		return fmt.Errorf("recording the skipped run of job %q: %w", jobName, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("reading the identifier of the skipped run of job %q: %w", jobName, err)
	}
	return s.updateJobState(ctx, id, Finish{Status: job.StatusSkipped, FinishedAt: at})
}

// updateJobState refreshes the cached state of the job a run belongs to.
func (s *Store) updateJobState(ctx context.Context, runID int64, finish Finish) error {
	var jobName, startedAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT job, started_at FROM runs WHERE id = ?`, runID).Scan(&jobName, &startedAt)
	if err != nil {
		return fmt.Errorf("reading the job of run %d: %w", runID, err)
	}

	stamp := formatTime(finish.FinishedAt)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO job_state (job, last_status, last_started_at, last_finished_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(job) DO UPDATE SET
			last_status = excluded.last_status,
			last_started_at = excluded.last_started_at,
			last_finished_at = excluded.last_finished_at,
			updated_at = excluded.updated_at`,
		jobName, string(finish.Status), startedAt, stamp, stamp)
	if err != nil {
		return fmt.Errorf("updating the state of job %q: %w", jobName, err)
	}
	return nil
}

// formatTime renders a timestamp for storage, in UTC.
func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

// parseTime reads a stored timestamp.
func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(timeFormat, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing the stored timestamp %q: %w", value, err)
	}
	return parsed, nil
}

// exitCodeValue converts an optional exit code into a nullable column value.
func exitCodeValue(code *int) any {
	if code == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*code), Valid: true}
}
