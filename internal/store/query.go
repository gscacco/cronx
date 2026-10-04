package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gscacco.com/cronx/internal/job"
)

// interruptReason explains why a run left in progress when the scheduler
// stopped was closed.
const interruptReason = "the scheduler stopped while this run was in progress"

// Runs returns executions in reverse chronological order, newest first. When
// jobName is empty every job is included. At most limit runs are returned.
func (s *Store) Runs(ctx context.Context, jobName string, limit int) ([]Run, error) {
	query := `SELECT ` + runColumns + ` FROM runs`
	var arguments []any
	if jobName != "" {
		query += ` WHERE job = ?`
		arguments = append(arguments, jobName)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	arguments = append(arguments, limit)

	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("reading the execution history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the execution history: %w", err)
	}
	return runs, nil
}

// RunningRuns counts the runs of a job that are still in progress.
func (s *Store) RunningRuns(ctx context.Context, jobName string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE job = ? AND status = ?`,
		jobName, string(job.StatusRunning)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting the runs of job %q in progress: %w", jobName, err)
	}
	return count, nil
}

// InterruptStaleRuns closes every run left in progress, which is what happens
// when the scheduler is restarted while jobs were running. It returns how many
// runs were closed.
func (s *Store) InterruptStaleRuns(ctx context.Context, at time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status = ?, finished_at = ?, error = ? WHERE status = ?`,
		string(job.StatusFailed), formatTime(at), interruptReason, string(job.StatusRunning))
	if err != nil {
		return 0, fmt.Errorf("closing the runs left in progress: %w", err)
	}

	interrupted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("counting the runs left in progress: %w", err)
	}
	return interrupted, nil
}

// scanner is implemented by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanRun reads one run from a query result.
func scanRun(row scanner) (Run, error) {
	var (
		run        Run
		status     string
		startedAt  string
		finishedAt sql.NullString
		exitCode   sql.NullInt64
		durationMS sql.NullInt64
		errorText  sql.NullString
		logPath    sql.NullString
	)

	err := row.Scan(&run.ID, &run.Job, &run.Attempt, &status, &startedAt,
		&finishedAt, &exitCode, &durationMS, &errorText, &logPath)
	if err != nil {
		return Run{}, fmt.Errorf("reading a run from the database: %w", err)
	}

	run.Status = job.Status(status)
	run.Error = errorText.String
	run.LogPath = logPath.String

	started, err := parseTime(startedAt)
	if err != nil {
		return Run{}, err
	}
	run.StartedAt = started

	if finishedAt.Valid {
		finished, err := parseTime(finishedAt.String)
		if err != nil {
			return Run{}, err
		}
		run.FinishedAt = finished
	}
	if exitCode.Valid {
		value := int(exitCode.Int64)
		run.ExitCode = &value
	}
	if durationMS.Valid {
		run.Duration = time.Duration(durationMS.Int64) * time.Millisecond
	}
	return run, nil
}
