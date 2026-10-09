// Package logx defines where cronx keeps its logs, how the output of a run is
// captured and how the scheduler's own log is written.
//
// Logs are files on disk: the scheduler log records what cronx itself did, and
// the output of every run of every job is written to a single log, where each
// line is prefixed with when it was written and with the process that produced
// it.
package logx

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gscacco.com/cronx/internal/clock"
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

// Log is the single file where the output of every run, of every job, is
// written. Every line carries a readable timestamp, the job, the run and the
// process that produced it.
//
// The file is created on the first write, so that a command which never runs a
// job leaves nothing behind. Runs may write to it concurrently: each line is
// written whole, and its bytes are never interleaved with those of another run.
// A configured Rotation renames the file once it would pass its maximum size,
// so that a job which prints a great deal cannot fill the disk.
type Log struct {
	path     string
	clock    clock.Clock
	rotation Rotation

	mu   sync.Mutex
	file *os.File
	// size is how much the file holds, so that a rotation can be decided
	// without asking the file system for every line.
	size int64
}

// Open returns the log whose output is written to path, stamping every line
// with the time read from clk and rotating the file as rotation says. Nothing
// is written where the log is returned: the file is created on the first write.
func Open(path string, clk clock.Clock, rotation Rotation) *Log {
	return &Log{path: path, clock: clk, rotation: rotation}
}

// Path returns the file the log is written to.
func (l *Log) Path() string {
	return l.path
}

// Writer returns the writer that captures the output of one run. It creates the
// log file, and its directory, when it is called for the first time. The caller
// must pass the identifier of the process to SetPID and call Close when the run
// ends.
func (l *Log) Writer(jobName string, runID int64) (*RunWriter, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.openLocked(); err != nil {
		return nil, err
	}
	return &RunWriter{log: l, job: jobName, id: runID}, nil
}

// Close closes the log. It is safe to call on a log that was never written to.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// openLocked creates the directory and the file of the log when they do not
// exist yet, and learns how much the file already holds. The caller holds the
// lock.
func (l *Log) openLocked() error {
	if l.file != nil {
		return nil
	}
	directory := filepath.Dir(l.path)
	if err := os.MkdirAll(directory, dirPermissions); err != nil {
		return fmt.Errorf("creating the log directory %s: %w", directory, err)
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermissions)
	if err != nil {
		return fmt.Errorf("opening the log %s: %w", l.path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("reading the size of the log %s: %w", l.path, err)
	}
	l.file = file
	l.size = info.Size()
	return nil
}

// write emits one line of a run, prefixed with what identifies it. A line that
// does not fit in the log any more rotates it first; a rotation that fails is
// not fatal, because capturing the output of a job is best effort and must not
// stop a run.
func (l *Log) write(prefix, text []byte) {
	entry := make([]byte, 0, len(prefix)+len(text)+1)
	entry = append(entry, prefix...)
	entry = append(entry, text...)
	entry = append(entry, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.openLocked(); err != nil {
		return
	}
	if l.rotation.rotates(l.size, int64(len(entry))) {
		if err := l.rotateLocked(); err != nil {
			return
		}
	}
	written, _ := l.file.Write(entry)
	l.size += int64(written)
}

// RunWriter captures the output of one run. It buffers what it receives until
// whole lines are available, then writes each of them to the shared log with
// the prefix that identifies the run.
//
// Lines written before the process identifier is known are held back, so that
// every line carries the identifier of the process that produced it.
type RunWriter struct {
	log *Log
	job string
	id  int64

	mu     sync.Mutex
	buffer []byte
	pid    int
	known  bool
}

// SetPID records the identifier of the process the output comes from.
func (w *RunWriter) SetPID(pid int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pid = pid
	w.known = true
	w.emit(false)
}

// Write buffers the output of the process. It never fails: capturing output is
// best effort and must not stop a job.
func (w *RunWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buffer = append(w.buffer, p...)
	if w.known {
		w.emit(false)
	}
	return len(p), nil
}

// Close flushes whatever is left, including a last line without a newline.
func (w *RunWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.known = true
	w.emit(true)
	return nil
}

// emit writes every whole line in the buffer. When final is true the trailing
// bytes, if any, are written as a line of their own. The caller holds the lock.
func (w *RunWriter) emit(final bool) {
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		w.log.write(w.prefix(), w.buffer[:index])
		w.buffer = w.buffer[index+1:]
	}
	if final && len(w.buffer) > 0 {
		w.log.write(w.prefix(), w.buffer)
		w.buffer = nil
	}
}

// prefix renders what every line of a run carries: when it was written, the
// job, the run and the process.
func (w *RunWriter) prefix() []byte {
	return fmt.Appendf(nil, "%s %s id=%d pid=%d ",
		w.log.clock.Now().Format(time.RFC3339), w.job, w.id, w.pid)
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
