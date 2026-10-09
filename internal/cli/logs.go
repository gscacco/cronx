package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/spf13/cobra"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/logx"
)

// followInterval is how often the run log is looked at again while it is being
// followed. An appended line is printed within it.
const followInterval = 250 * time.Millisecond

// newLogsCommand builds the "logs" command, which prints the run log: the
// output of every run, as the scheduler recorded it.
func newLogsCommand(configPath *string) *cobra.Command {
	var (
		follow bool
		since  string
	)

	command := &cobra.Command{
		Use:   "logs [job]",
		Short: "Print the output the runs wrote to the run log",
		Long: "Print the run log, oldest line first.\n\n" +
			"With a job name only the lines of that job are shown; without one the\n" +
			"lines of every job are. --since leaves out the lines written before a\n" +
			"duration ago, or before an instant; --follow keeps printing as the log\n" +
			"grows, until the command is stopped.\n\n" +
			"The run log is read, never written: no state is opened and nothing is\n" +
			"created. When no line matches, nothing is printed, so the command can be\n" +
			"piped or redirected.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}

			path, err := config.ResolvePath(*configPath)
			if err != nil {
				return err
			}
			configuration, err := config.Load(path)
			if err != nil {
				return err
			}
			logPath, err := config.LogPath(configuration.Logging.Path)
			if err != nil {
				return err
			}
			cutoff, err := parseSince(since, time.Now())
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			filter := lineFilter{job: name, since: cutoff}
			offset, read, err := printRunLog(out, logPath, filter)
			if err != nil || !follow {
				return err
			}
			return followRunLog(cmd.Context(), out, logPath, offset, read, filter)
		},
	}

	command.Flags().BoolVar(&follow, "follow", false,
		"keep printing the lines as they are written")
	command.Flags().StringVar(&since, "since", "",
		"only the lines written within a duration ago (30m, 2h) or after an RFC 3339 instant")
	return command
}

// lineFilter selects the lines of the run log that are of interest.
type lineFilter struct {
	// job is the job whose lines are wanted. Empty means every job.
	job string
	// since is the oldest instant a line may carry. The zero time means no
	// limit.
	since time.Time
}

// matches reports whether a line belongs to what was asked for.
func (f lineFilter) matches(entry logx.Entry) bool {
	if f.job != "" && entry.Job != f.job {
		return false
	}
	return f.since.IsZero() || !entry.Time.Before(f.since)
}

// parseSince reads the --since value: a duration counted back from now, or an
// instant. An empty value means no limit.
func parseSince(value string, now time.Time) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if duration, err := time.ParseDuration(value); err == nil {
		if duration < 0 {
			return time.Time{}, fmt.Errorf("--since %q is negative: it counts back from now", value)
		}
		return now.Add(-duration), nil
	}
	instant, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"--since %q is neither a duration such as 30m nor an RFC 3339 instant such as 2026-01-02T03:04:05Z",
			value)
	}
	return instant, nil
}

// printRunLog prints the lines of the run log the filter selects and returns
// how much of the file was read and which file it was, so that a follower can
// tell an append from a rotation. A log that does not exist yet is not an
// error: it holds no line.
func printRunLog(out io.Writer, path string, filter lineFilter) (int64, os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil, nil
		}
		return 0, nil, fmt.Errorf("reading the run log %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return 0, nil, fmt.Errorf("reading the run log %s: %w", path, err)
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return 0, nil, fmt.Errorf("reading the run log %s: %w", path, err)
	}
	printLines(out, content, filter)
	return int64(len(content)), info, nil
}

// printLines prints the complete lines of a block of log text that the filter
// selects, and returns the trailing bytes: those do not end in a newline, so
// they are left for a later append to complete.
func printLines(out io.Writer, content []byte, filter lineFilter) []byte {
	for {
		index := bytes.IndexByte(content, '\n')
		if index < 0 {
			return content
		}
		line := string(content[:index])
		content = content[index+1:]

		entry, err := logx.ParseLine(line)
		if err != nil || !filter.matches(entry) {
			continue
		}
		fmt.Fprintln(out, line)
	}
}

// followRunLog keeps printing the lines appended to the run log until the
// command is stopped. The offset is how much of the file was already read.
//
// A log that is rotated — renamed away and replaced by an empty file — is
// recognised by comparing the file at the path with the one that was read
// before, and reading starts again from the beginning of the new one, so that
// what is written after a rotation is printed too.
func followRunLog(ctx context.Context, out io.Writer, path string, offset int64, read os.FileInfo, filter lineFilter) error {
	var pending []byte

	ticker := time.NewTicker(followInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("reading the run log %s: %w", path, err)
		}
		switch {
		case read == nil || !os.SameFile(read, info):
			// The log was created or replaced by a new one.
			offset = 0
			pending = nil
		case info.Size() < offset:
			// The file was made shorter in place.
			offset = 0
			pending = nil
		}
		read = info
		if info.Size() == offset {
			continue
		}

		content, err := readFrom(path, offset)
		if err != nil {
			return err
		}
		offset += int64(len(content))
		pending = printLines(out, append(pending, content...), filter)
	}
}

// readFrom reads the run log from the given offset to its end.
func readFrom(path string, offset int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading the run log %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("reading the run log %s: %w", path, err)
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("reading the run log %s: %w", path, err)
	}
	return content, nil
}
