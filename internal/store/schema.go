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

// querier is the part of a database handle a version can be read through: the
// pool, or the transaction that is about to migrate the schema.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SchemaVersion returns the schema version stored in the database.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	return storedVersion(ctx, s.db)
}

// migrate brings the database up to CurrentSchemaVersion.
func (s *Store) migrate(ctx context.Context) error {
	version, err := storedVersion(ctx, s.db)
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
// when the database has not been initialised yet. It reads through the given
// handle, so that the version can also be checked inside the transaction that
// is about to migrate the schema.
func storedVersion(ctx context.Context, q querier) (int, error) {
	var tables int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_meta'`).Scan(&tables)
	if err != nil {
		return 0, fmt.Errorf("inspecting the database schema: %w", err)
	}
	if tables == 0 {
		return 0, nil
	}

	var version int
	err = q.QueryRowContext(ctx, `SELECT version FROM schema_meta LIMIT 1`).Scan(&version)
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
//
// The version is read again inside the transaction, which is serialised against
// the other processes by the immediate transaction the connection begins. A
// process that finds the migration already applied — because another one
// applied it in the meantime — does nothing instead of failing with "table
// schema_meta already exists".
func (s *Store) applyMigration(ctx context.Context, version int) error {
	return s.inTransaction(ctx, func(tx *sql.Tx) error {
		applied, err := storedVersion(ctx, tx)
		if err != nil {
			return err
		}
		if applied >= version {
			return nil
		}
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
