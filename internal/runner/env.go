package runner

import (
	"sort"
	"strings"
)

// DefaultPath is the PATH given to every job.
//
// cronx never uses it to resolve a job's own command, which must be an absolute
// path; it exists only so that the programs a job starts can find their own
// helpers. A job that needs a different PATH declares it in its `env`.
const DefaultPath = "/usr/local/bin:/usr/bin:/bin"

// BaseEnvironment returns the minimal environment given to every job.
//
// The scheduler's own environment is deliberately not inherited: this keeps
// secrets and unrelated settings out of the jobs. Jobs add whatever else they
// need through their `env` table.
func BaseEnvironment() []string {
	return []string{"PATH=" + DefaultPath}
}

// environment builds the environment for a process from the minimal base
// environment and the job's own variables, which take precedence. The result is
// sorted so that it is stable.
func environment(overrides map[string]string) []string {
	values := make(map[string]string, len(overrides)+1)
	for _, entry := range BaseEnvironment() {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	for name, value := range overrides {
		values[name] = value
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]string, 0, len(names))
	for _, name := range names {
		entries = append(entries, name+"="+values[name])
	}
	return entries
}
