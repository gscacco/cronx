// Package logx defines where cronx keeps its logs, how the output of a run is
// captured and how the scheduler's own log is written.
//
// Logs are files on disk: the scheduler log records what cronx itself did, and
// each run gets its own file holding the output of the job.
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Permissions used for the log tree. Logs can contain anything a job prints, so
// only their owner may read them.
const (
	dirPermissions  = 0o700
	filePermissions = 0o600
)

// homeDirectoryName is the directory, relative to the user's home, that holds
// the configuration, the state and the logs.
const homeDirectoryName = ".cronx"

// logsDirectoryName is the name of the log directory inside the home directory.
const logsDirectoryName = "logs"

// schedulerLogName is the name of the scheduler's own log file.
const schedulerLogName = "cronx.log"

// Layout is the directory tree that holds the logs.
type Layout struct {
	root string
}

// NewLayout returns the layout rooted at the given directory.
func NewLayout(root string) Layout {
	return Layout{root: root}
}

// DefaultLayout returns the layout used when nothing else is configured:
// ~/.cronx/logs.
func DefaultLayout() (Layout, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Layout{}, fmt.Errorf("determining the user home directory: %w", err)
	}
	return NewLayout(filepath.Join(home, homeDirectoryName, logsDirectoryName)), nil
}

// Root returns the directory the logs live under.
func (l Layout) Root() string {
	return l.root
}

// SchedulerPath returns the path of the scheduler's own log file.
func (l Layout) SchedulerPath() string {
	return filepath.Join(l.root, schedulerLogName)
}

// RunPath returns the path of the log file of one run of a job.
func (l Layout) RunPath(jobName string, runID int64) string {
	return filepath.Join(l.root, jobName, fmt.Sprintf("%d.log", runID))
}

// CreateRunFile creates the log file for a run, replacing any previous content.
// The caller owns the returned file and must close it.
func (l Layout) CreateRunFile(jobName string, runID int64) (*os.File, error) {
	path := l.RunPath(jobName, runID)
	if err := os.MkdirAll(filepath.Dir(path), dirPermissions); err != nil {
		return nil, fmt.Errorf("creating the log directory of job %q: %w", jobName, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePermissions)
	if err != nil {
		return nil, fmt.Errorf("creating the log file %s: %w", path, err)
	}
	return file, nil
}

// OpenSchedulerLog opens the scheduler's log for appending. The caller owns the
// returned file and must close it.
func (l Layout) OpenSchedulerLog() (*os.File, error) {
	if err := os.MkdirAll(l.root, dirPermissions); err != nil {
		return nil, fmt.Errorf("creating the log directory %s: %w", l.root, err)
	}

	path := l.SchedulerPath()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermissions)
	if err != nil {
		return nil, fmt.Errorf("opening the scheduler log %s: %w", path, err)
	}
	return file, nil
}

// NewLogger returns a logger that writes the given level and above to w. The
// accepted levels are those the configuration allows: debug, info, warn, error.
func NewLogger(w io.Writer, level string) (*slog.Logger, error) {
	parsed, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: parsed})), nil
}

// parseLevel maps a configured level name onto a slog level.
func parseLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown logging level %q", level)
	}
}
