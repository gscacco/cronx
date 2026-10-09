package logx

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// entryFields is how many parts a line of the run log is read as: the instant,
// the job, the run, the pid and the message, the last of which holds whatever
// the process printed, spaces included.
const entryFields = 5

// Entry is one line of the shared run log, split into the fields its prefix
// carries.
type Entry struct {
	// Time is the instant the line was written at.
	Time time.Time
	// Job is the name of the job whose run produced the line.
	Job string
	// RunID is the run the line belongs to: the identifier "cronx history"
	// reports.
	RunID int64
	// PID is the process the line came from.
	PID int
	// Message is what the process printed, without the prefix.
	Message string
}

// ParseLine reads one line of the shared run log, as the run writer writes it:
// an RFC 3339 instant, the job, the run, the pid, and then the output of the
// process. It fails on anything that is not such a line, so that a reader can
// tell the run log from a file that is not one.
func ParseLine(line string) (Entry, error) {
	fields := strings.SplitN(line, " ", entryFields)
	if len(fields) < entryFields-1 {
		return Entry{}, fmt.Errorf(
			"a run log line has %d fields, want the instant, the job, the run, the pid and the output",
			len(fields))
	}

	at, err := time.Parse(time.RFC3339, fields[0])
	if err != nil {
		return Entry{}, fmt.Errorf("the instant %q of a run log line is not an RFC 3339 timestamp", fields[0])
	}
	if fields[1] == "" {
		return Entry{}, fmt.Errorf("a run log line does not name the job")
	}

	runID, err := parseKeyed(fields[2], "id")
	if err != nil {
		return Entry{}, err
	}
	pid, err := parseKeyed(fields[3], "pid")
	if err != nil {
		return Entry{}, err
	}

	message := ""
	if len(fields) == entryFields {
		message = fields[4]
	}
	return Entry{Time: at, Job: fields[1], RunID: runID, PID: int(pid), Message: message}, nil
}

// parseKeyed reads the number of a "key=value" field of a run log line.
func parseKeyed(field, key string) (int64, error) {
	prefix := key + "="
	if !strings.HasPrefix(field, prefix) {
		return 0, fmt.Errorf("the field %q of a run log line is not %s=<number>", field, key)
	}
	value, err := strconv.ParseInt(field[len(prefix):], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("the %s of a run log line is not a number: %q", key, field)
	}
	return value, nil
}
