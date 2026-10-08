// Package store persists the runtime state and the execution history of cronx.
//
// The configuration remains the single source of truth for the desired state;
// the database holds what actually happened, and the bookkeeping the scheduler
// needs in order to apply its policies.
//
// The schema is versioned and upgraded automatically when the database is
// opened.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	// The pure Go SQLite driver: no cgo, so cronx stays portable and easy to
	// cross-compile.
	_ "modernc.org/sqlite"
)

// driverName is the name registered by the SQLite driver.
const driverName = "sqlite"

// directoryPermissions is the mode used for the directory holding the database:
// only its owner may read it.
const directoryPermissions = 0o700

// Store is a handle to the cronx database.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite database at path, creating the file and its parent
// directory when needed, then applies the schema migrations.
func Open(path string) (*Store, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, directoryPermissions); err != nil {
		return nil, fmt.Errorf("creating the directory %s: %w", directory, err)
	}

	db, err := sql.Open(driverName, dataSourceName(path))
	if err != nil {
		return nil, fmt.Errorf("opening the database %s: %w", path, err)
	}
	return setup(db)
}

// OpenMemory opens a private in-memory database and applies the schema. It is
// intended for tests.
func OpenMemory() (*Store, error) {
	db, err := sql.Open(driverName, ":memory:")
	if err != nil {
		return nil, fmt.Errorf("opening an in-memory database: %w", err)
	}
	// An in-memory database lives in a single connection.
	db.SetMaxOpenConns(1)
	return setup(db)
}

// setup applies the schema and closes the database if that fails.
func setup(db *sql.DB) (*Store, error) {
	subject := &Store{db: db}
	if err := subject.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return subject, nil
}

// Close releases the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// dataSourceName builds the connection string for a database file. The pragmas
// make concurrent access wait rather than fail, keep the write-ahead log on and
// enforce foreign keys. Transactions begin immediately, so that two processes
// migrating a brand new database take turns instead of both deciding to apply
// the same migration.
func dataSourceName(path string) string {
	return "file:" + path +
		"?_txlock=immediate" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)"
}
