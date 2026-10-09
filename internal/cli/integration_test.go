package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// The integration tests drive the commands against the configuration files
// that ship with the project, so that the examples a person reads are exactly
// the files the test suite checks.

// examplesDirectory holds the reference configurations, relative to the
// directory of this package.
const examplesDirectory = "../../examples/configs"

// brokenExample is the one reference configuration that is meant to be
// rejected; every mistake it makes on purpose is listed in brokenProblems, in
// diagnostics_test.go.
const brokenExample = "broken.toml"

// listTimestampLayout is how the NEXT column of "list" renders an instant.
const listTimestampLayout = "2006-01-02 15:04:05 MST"

// referenceJob is a job a reference configuration defines, as "list" reports
// it. An empty next means a computed instant; "-" means the job never runs.
type referenceJob struct {
	name     string
	schedule string
	next     string
}

// referenceExamples are the configurations a person is expected to copy and
// adapt. Each of them must validate and list exactly the jobs below.
var referenceExamples = []struct {
	file string
	jobs []referenceJob
}{
	{
		file: "minimal.toml",
		jobs: []referenceJob{
			{name: "hello", schedule: "*/5 * * * *"},
		},
	},
	{
		file: "full.toml",
		jobs: []referenceJob{
			{name: "archiver", schedule: "0 5 * * 0", next: disabledNext},
			{name: "backup", schedule: "0 3 * * *"},
			{name: "leap-check", schedule: "0 0 31 4 *", next: "-"},
			{name: "metrics", schedule: "*/5 * * * *"},
			{name: "reindex", schedule: "30 2 * * 6"},
		},
	},
	{
		file: "maintenance.toml",
		jobs: []referenceJob{
			{name: "backup", schedule: "0 3 * * *"},
			{name: "logrotate", schedule: "5 * * * *"},
			{name: "report", schedule: "0 7 * * 1-5"},
			{name: "vacuum", schedule: "0 4 * * 0"},
		},
	},
	{
		file: "descriptors.toml",
		jobs: []referenceJob{
			{name: "yearly", schedule: "@yearly"},
			{name: "monthly", schedule: "@monthly"},
			{name: "weekly", schedule: "@weekly"},
			{name: "daily", schedule: "@daily"},
			{name: "midnight", schedule: "@midnight"},
			{name: "hourly", schedule: "@hourly"},
			{name: "reboot", schedule: "@reboot", next: startupNext},
		},
	},
	{
		file: "seconds.toml",
		jobs: []referenceJob{
			{name: "heartbeat", schedule: "*/10 * * * * *"},
			{name: "poller", schedule: "0,30 * * * * *"},
		},
	},
	{
		file: "operators.toml",
		jobs: []referenceJob{
			{name: "cleanup", schedule: "0 22 * * 5L"},
			{name: "invoice", schedule: "0 9 15W * *"},
			{name: "month-close", schedule: "0 0 L * *"},
			{name: "payroll", schedule: "0 6 1,15,L * *"},
			{name: "report", schedule: "0 7 LW * *"},
			{name: "rotation", schedule: "30 3 * * sun#1"},
			{name: "sweep", schedule: "0 4 * * L"},
		},
	},
}

// examplePath returns the path of a reference configuration.
func examplePath(file string) string {
	return filepath.Join(examplesDirectory, file)
}

// listRow returns the fields of the "list" row that describes a job.
func listRow(t *testing.T, output, job string) []string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == job {
			return fields
		}
	}
	t.Fatalf("list output = %q, want a row for the job %q", output, job)
	return nil
}

// listColumns splits a row of "list" into the schedule and what the NEXT column
// says: an instant, shown as a date, a time and a zone, the marker of a job that
// runs when the scheduler starts, or a single dash for a job that never runs. A
// row carries no separators, so the expression the job is expected to have is
// what says where the schedule ends: five fields, six with the seconds field in
// front of them, or one descriptor, which is a field on its own. Whatever follows
// it is the NEXT column, which checkNext reads.
func listColumns(t *testing.T, fields []string, expression string) (schedule, next string) {
	t.Helper()
	width := len(strings.Fields(expression))
	if len(fields) < 1+width+1 {
		t.Fatalf("list row = %v, want a job, a schedule and a NEXT column", fields)
	}
	schedule = strings.Join(fields[1:1+width], " ")
	return schedule, strings.Join(fields[1+width:], " ")
}

// startupNext is what the NEXT column says of a job that runs when the
// scheduler starts, in place of an instant.
const startupNext = "at startup"

// disabledNext is what the NEXT column says of a job the configuration keeps
// without scheduling it. It is a third kind of answer: not an instant, not a
// schedule that can never match, and not a job waiting for a start.
const disabledNext = "disabled"

// checkNext verifies the NEXT column of a job: the marker of a disabled job,
// the marker of a job that runs when the scheduler starts, a dash when the
// expression can never match, and an instant otherwise.
func checkNext(t *testing.T, job referenceJob, next string) {
	t.Helper()
	switch job.next {
	case disabledNext:
		if next != disabledNext {
			t.Errorf("%s runs next %q, want %q because the job is disabled",
				job.name, next, disabledNext)
		}
		return
	case startupNext:
		if next != startupNext {
			t.Errorf("%s runs next %q, want %q because it runs when the scheduler starts",
				job.name, next, startupNext)
		}
		return
	case "-":
		if next != "-" {
			t.Errorf("%s runs next at %q, want a dash because the schedule never matches",
				job.name, next)
		}
		return
	}
	if _, err := time.Parse(listTimestampLayout, next); err != nil {
		t.Errorf("%s runs next at %q, want an instant: %v", job.name, next, err)
	}
}

func TestReferenceConfigurationsAreValid(t *testing.T) {
	// SETUP
	newTestHome(t)

	for _, example := range referenceExamples {
		t.Run(example.file, func(t *testing.T) {
			// EXERCISE
			output, err := runCLI(t, "validate", "--config", examplePath(example.file))

			// VERIFY
			if err != nil {
				t.Fatalf("validate returned an unexpected error: %v", err)
			}
			if !strings.Contains(output, "is valid") {
				t.Errorf("validate output = %q, want it to report the file as valid", output)
			}
		})
	}
}

func TestReferenceConfigurationsListTheirJobs(t *testing.T) {
	// SETUP
	newTestHome(t)

	for _, example := range referenceExamples {
		t.Run(example.file, func(t *testing.T) {
			// EXERCISE
			output, err := runCLI(t, "list", "--config", examplePath(example.file))

			// VERIFY
			if err != nil {
				t.Fatalf("list returned an unexpected error: %v", err)
			}
			for _, job := range example.jobs {
				schedule, next := listColumns(t, listRow(t, output, job.name), job.schedule)
				if schedule != job.schedule {
					t.Errorf("%s: schedule = %q, want %q", job.name, schedule, job.schedule)
				}
				checkNext(t, job, next)
			}
		})
	}
}

func TestEveryReferenceConfigurationIsCheckedByTheTests(t *testing.T) {
	// SETUP
	known := []string{brokenExample}
	for _, example := range referenceExamples {
		known = append(known, example.file)
	}
	sort.Strings(known)

	// EXERCISE
	entries, err := os.ReadDir(examplesDirectory)
	if err != nil {
		t.Fatalf("reading %s: %v", examplesDirectory, err)
	}
	found := make([]string, 0, len(entries))
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".toml" {
			found = append(found, entry.Name())
		}
	}
	sort.Strings(found)

	// VERIFY
	if !slices.Equal(found, known) {
		t.Errorf("examples/configs holds %v, want exactly the files the tests know about: %v",
			found, known)
	}
}
