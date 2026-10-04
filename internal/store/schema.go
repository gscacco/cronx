package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CurrentSchemaVersion is the schema version this build understands. It always
// equals the number of migrations below.
const CurrentSchemaVersion = 1

// migrations are applied in order; the index plus one is the resulting schema
// version.
var migrations = []string{
	// Version 1: the initial schema.
	`
CREATE TABLE schema_meta (
	version INTEGER NOT NULL
);

CREATE TABLE runs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	job         TEXT    NOT NULL,
	attempt     INTEGER NOT NULL,
	status      TEXT    NOT NULL,
	started_at  TEXT    NOT NULL,
	finished_at TEXT,
	exit_code   INTEGER,
	duration_ms INTEGER,
	error       TEXT,
	log_path    TEXT
);

CREATE INDEX runs_by_job ON runs (job, id DESC);

CREATE TABLE job_state (
	job              TEXT PRIMARY KEY,
	last_status      TEXT,
	last_started_at  TEXT,
	last_finished_at TEXT,
	updated_at       TEXT NOT NULL
);
`,
}

// SchemaVersion returns the schema version stored in the database.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	return s.storedVersion(ctx)
}

// migrate brings the database up to CurrentSchemaVersion.
func (s *Store) migrate(ctx context.Context) error {
	version, err := s.storedVersion(ctx)
	if err != nil {
		return err
	}
	if version > CurrentSchemaVersion {
		return fmt.Errorf(
			"the database schema version %d is newer than the supported version %d",
			version, CurrentSchemaVersion)
	}

	for next := version + 1; next <= len(migrations); next++ {
		if err := s.applyMigration(ctx, next); err != nil {
			return err
		}
	}
	return nil
}

// storedVersion returns the schema version recorded in the database, or zero
// when the database has not been initialised yet.
func (s *Store) storedVersion(ctx context.Context) (int, error) {
	var tables int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_meta'`).Scan(&tables)
	if err != nil {
		return 0, fmt.Errorf("inspecting the database schema: %w", err)
	}
	if tables == 0 {
		return 0, nil
	}

	var version int
	err = s.db.QueryRowContext(ctx, `SELECT version FROM schema_meta LIMIT 1`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading the schema version: %w", err)
	}
	return version, nil
}

// applyMigration runs the migration with the given version and records it, all
// in one transaction so that a failure leaves the database untouched.
func (s *Store) applyMigration(ctx context.Context, version int) error {
	return s.inTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, migrations[version-1]); err != nil {
			return fmt.Errorf("applying schema version %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM schema_meta`); err != nil {
			return fmt.Errorf("updating the schema version: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_meta (version) VALUES (?)`, version); err != nil {
			return fmt.Errorf("recording schema version %d: %w", version, err)
		}
		return nil
	})
}

// inTransaction runs fn inside a transaction, rolling it back on error.
func (s *Store) inTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting a transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing a transaction: %w", err)
	}
	return nil
}
