package logx

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Rotation keeps the run log from growing without bound: when the log would
// pass MaxSize it is renamed to <path>.1, the file before it to <path>.2 and so
// on, and a new empty log is started. At most MaxBackups of the renamed files
// are kept, and the next rotation drops the oldest of them.
//
// The zero value rotates nothing: the log grows without limit, which is what an
// installation that says nothing about rotation asks for.
type Rotation struct {
	// MaxSize is how many bytes the log may reach before it is rotated.
	MaxSize int64
	// MaxBackups is how many rotated files are kept.
	MaxBackups int
}

// rotates reports whether a line of the given size, written into a log that
// already holds current bytes, calls for a rotation.
//
// A log is never rotated before its first line, so a line larger than MaxSize
// is written rather than rotated away: a job that prints more at once than the
// log may hold would otherwise leave nothing behind at all.
func (r Rotation) rotates(current, line int64) bool {
	if r.MaxSize <= 0 || current == 0 {
		return false
	}
	return current+line > r.MaxSize
}

// backupPath is the name of the index-th rotated file of a log: the path of the
// log with ".1", ".2" and so on appended. The lower the index, the newer the
// file.
func backupPath(path string, index int) string {
	return fmt.Sprintf("%s.%d", path, index)
}

// rotateLocked renames the log and starts a new empty one. The caller holds the
// lock of the log.
func (l *Log) rotateLocked() error {
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("closing the log %s to rotate it: %w", l.path, err)
	}
	l.file = nil
	l.size = 0

	if l.rotation.MaxBackups < 1 {
		// No rotated file is kept: the lines written so far are dropped.
		if err := os.Remove(l.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing the log %s to rotate it: %w", l.path, err)
		}
		return l.openLocked()
	}

	// The oldest file is dropped, the others move one step back, and the log
	// itself becomes the newest of them.
	if err := os.Remove(backupPath(l.path, l.rotation.MaxBackups)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing the oldest rotated log of %s: %w", l.path, err)
	}
	for index := l.rotation.MaxBackups - 1; index >= 1; index-- {
		from := backupPath(l.path, index)
		switch _, err := os.Stat(from); {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return fmt.Errorf("looking at the rotated log %s: %w", from, err)
		}
		if err := os.Rename(from, backupPath(l.path, index+1)); err != nil {
			return fmt.Errorf("moving the rotated log %s: %w", from, err)
		}
	}
	if err := os.Rename(l.path, backupPath(l.path, 1)); err != nil {
		return fmt.Errorf("renaming the log %s to rotate it: %w", l.path, err)
	}
	return l.openLocked()
}
