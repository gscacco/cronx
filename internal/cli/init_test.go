package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/config"
)

// generatedPath is where "cronx init" writes when nothing selects another file:
// the configuration path every command falls back to, inside the home directory
// of a test.
func generatedPath(home string) string {
	return filepath.Join(home, ".cronx", "config.toml")
}

// modeOf returns the permission bits of a path.
func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("looking at %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// The two kinds of line a configuration is made of: the one that opens a table
// and the one that sets an option. Both are recognised whether or not the line
// is commented out, because the file cronx writes documents its options by
// commenting them out.
var (
	tableLine  = regexp.MustCompile(`^\[([^]]+)\]`)
	optionLine = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*=`)
)

// section returns the table a line opens, if it opens one.
func section(line string) (string, bool) {
	match := tableLine.FindStringSubmatch(line)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// option returns the option a line sets, if it sets one.
func option(line string) (string, bool) {
	match := optionLine.FindStringSubmatch(line)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// uncommented returns a line without its comment marker, so that a line that
// only documents an option reads as one that sets it.
func uncommented(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
}

// optionNames returns the tables and the options a configuration text mentions,
// commented out or set, so that the file cronx writes can be compared with the
// reference configuration. A job table is reported as "jobs.<name>", whatever
// the job is called.
func optionNames(text string) map[string]bool {
	names := make(map[string]bool)
	for _, line := range strings.Split(text, "\n") {
		line = uncommented(line)
		if name, ok := section(line); ok {
			if strings.HasPrefix(name, "jobs.") {
				name = "jobs.<name>"
			}
			names[name] = true
			continue
		}
		if name, ok := option(line); ok {
			names[name] = true
		}
	}
	return names
}

// everyOptionSet rewrites a configuration as it would read with every option it
// documents turned on: the prose is dropped, each commented option is set, and
// an option the file already sets keeps the value the file sets, since the
// commented line documents that option rather than adding it.
func everyOptionSet(text string) string {
	var (
		kept  []string
		table string
		set   = make(map[string]bool)
	)
	for _, line := range strings.Split(text, "\n") {
		stripped := strings.TrimSpace(line)
		documented := strings.HasPrefix(stripped, "#")
		stripped = uncommented(stripped)

		if name, ok := section(stripped); ok {
			if documented {
				continue // the table is already in the file
			}
			table = name
			kept = append(kept, line)
			continue
		}
		name, ok := option(stripped)
		if !ok {
			continue // prose
		}
		if !documented {
			set[table+"."+name] = true
			kept = append(kept, line)
			continue
		}
		if set[table+"."+name] {
			continue // the file sets this option itself
		}
		kept = append(kept, stripped)
	}
	return strings.Join(kept, "\n")
}

func TestInitWritesAConfigurationToStartFrom(t *testing.T) {
	// SETUP
	home := newTestHome(t)

	// EXERCISE
	output, err := runCLI(t, "init")

	// VERIFY: the file was written where the commands look for it, it is
	// private, and it is a configuration cronx accepts.
	path := generatedPath(home)
	if err != nil {
		t.Fatalf("init returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, path) {
		t.Errorf("init output = %q, want it to name %s", output, path)
	}
	if mode := modeOf(t, path); mode != 0o600 {
		t.Errorf("the generated configuration is %o, want 0600", mode)
	}
	if mode := modeOf(t, filepath.Dir(path)); mode != 0o700 {
		t.Errorf("the generated directory is %o, want 0700", mode)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatalf("the generated configuration is not valid: %v", err)
	}

	// VERIFY: writing that file is all the command did: no state was opened
	// and no log was created.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading the configuration directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.toml" {
		t.Errorf("the configuration directory holds %v, want only the file init wrote", entries)
	}
}

func TestTheGeneratedConfigurationCanBeValidatedAndListed(t *testing.T) {
	// SETUP
	newTestHome(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init returned an unexpected error: %v", err)
	}

	// EXERCISE
	validated, err := runCLI(t, "validate")
	if err != nil {
		t.Fatalf("validate returned an unexpected error: %v", err)
	}
	listed, err := runCLI(t, "list")
	if err != nil {
		t.Fatalf("list returned an unexpected error: %v", err)
	}

	// VERIFY: the file init writes is what the other commands read when
	// nothing selects another one.
	if !strings.Contains(validated, "is valid") {
		t.Errorf("validate output = %q, want it to report the generated file as valid", validated)
	}
	for _, want := range []string{"hello", "*/5 * * * *"} {
		if !strings.Contains(listed, want) {
			t.Errorf("list output = %q, want it to contain %q", listed, want)
		}
	}
}

func TestInitRefusesToReplaceAConfigurationThatExists(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)
	before := readFile(t, path)

	// EXERCISE
	_, err := runCLI(t, "init")

	// VERIFY: the file is named, the way out is named with it, and the file
	// itself is left as it was.
	if err == nil {
		t.Fatalf("init succeeded, want it to refuse to replace a configuration that exists")
	}
	for _, want := range []string{path, "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("init error = %q, want it to contain %q", err.Error(), want)
		}
	}
	if after := readFile(t, path); after != before {
		t.Errorf("the configuration is now %q, want it left as it was:\n%s", after, before)
	}
}

func TestInitReplacesTheFileWhenItIsAskedTo(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	path := writeTestConfig(t, home, twoJobs)

	// EXERCISE
	_, err := runCLI(t, "init", "--force")

	// VERIFY: the file is the one init writes, and the jobs that were there
	// are gone.
	if err != nil {
		t.Fatalf("init --force returned an unexpected error: %v", err)
	}
	configuration, err := config.Load(path)
	if err != nil {
		t.Fatalf("the configuration init wrote is not valid: %v", err)
	}
	if _, ok := configuration.Jobs["hello"]; !ok {
		t.Errorf("the configuration holds %v, want the job init writes", configuration.Jobs)
	}
	for _, replaced := range []string{"backup", "cleanup"} {
		if _, ok := configuration.Jobs[replaced]; ok {
			t.Errorf("the configuration still holds the job %q it had:\n%s",
				replaced, readFile(t, path))
		}
	}
}

func TestInitWritesWhereTheConfigurationIsLookedFor(t *testing.T) {
	// SETUP: two files, each in a directory that does not exist yet.
	home := newTestHome(t)
	fromFlag := filepath.Join(t.TempDir(), "nested", "jobs.toml")
	fromVariable := filepath.Join(t.TempDir(), "from-variable.toml")

	// EXERCISE: the flag selects the file, and the directory it names is
	// created with it.
	byFlag, err := runCLI(t, "init", "--config", fromFlag)
	if err != nil {
		t.Fatalf("init --config returned an unexpected error: %v", err)
	}

	// EXERCISE: CRONX_CONFIG selects the file when no flag does.
	t.Setenv(config.EnvVar, fromVariable)
	byVariable, err := runCLI(t, "init")
	if err != nil {
		t.Fatalf("init returned an unexpected error: %v", err)
	}

	// VERIFY
	for _, written := range []struct {
		output string
		path   string
	}{
		{output: byFlag, path: fromFlag},
		{output: byVariable, path: fromVariable},
	} {
		if !strings.Contains(written.output, written.path) {
			t.Errorf("init output = %q, want it to name %s", written.output, written.path)
		}
		if _, err := config.Load(written.path); err != nil {
			t.Errorf("the configuration at %s is not valid: %v", written.path, err)
		}
	}
	if mode := modeOf(t, filepath.Dir(fromFlag)); mode != 0o700 {
		t.Errorf("the directory init created is %o, want 0700", mode)
	}
	if _, err := os.Stat(generatedPath(home)); err == nil {
		t.Errorf("the default configuration %s was written, want only the file that was asked for",
			generatedPath(home))
	}
}

func TestTheGeneratedConfigurationDocumentsEveryOption(t *testing.T) {
	// SETUP
	home := newTestHome(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init returned an unexpected error: %v", err)
	}
	generated := readFile(t, generatedPath(home))

	// EXERCISE
	documented := optionNames(generated)

	// VERIFY: every option the reference configuration uses is documented, so
	// that a key added to the schema and to examples/configs/full.toml cannot
	// be missing here.
	wanted := optionNames(readFile(t, filepath.Join(examplesDirectory, "full.toml")))
	// full.toml leaves the two optional paths out on purpose — the suite
	// validates it as it is, and they would point outside the home directory —
	// so the generated file is what has to document them.
	wanted["path"] = true
	for name := range wanted {
		if !documented[name] {
			t.Errorf("the generated configuration does not document %q:\n%s", name, generated)
		}
	}

	// VERIFY: the job the file defines is the one its own prose describes.
	configuration, err := config.Load(generatedPath(home))
	if err != nil {
		t.Fatalf("the generated configuration is not valid: %v", err)
	}
	if len(configuration.Jobs) != 1 {
		t.Fatalf("the generated configuration defines %d jobs, want exactly one", len(configuration.Jobs))
	}
	hello, ok := configuration.Jobs["hello"]
	if !ok {
		t.Fatalf("the generated configuration defines %v, want a job named hello", configuration.Jobs)
	}
	if hello.Schedule != "*/5 * * * *" {
		t.Errorf("the schedule of the generated job is %q, want %q", hello.Schedule, "*/5 * * * *")
	}
}

func TestTheGeneratedConfigurationIsValidWithEveryOptionSet(t *testing.T) {
	// SETUP: the file cronx writes, with every option it documents turned on.
	// What it explains is a configuration of its own, so it has to be one cronx
	// accepts: no option that does not exist, no value that is refused.
	home := newTestHome(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init returned an unexpected error: %v", err)
	}
	path := filepath.Join(t.TempDir(), "every-option.toml")
	if err := os.WriteFile(path, []byte(everyOptionSet(readFile(t, generatedPath(home)))), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	// EXERCISE
	output, err := runCLI(t, "validate", "--config", path)

	// VERIFY
	if err != nil {
		t.Fatalf("a configuration with every documented option set is not valid: %v\n%s",
			err, readFile(t, path))
	}
	if !strings.Contains(output, "is valid") {
		t.Errorf("validate output = %q, want it to report the configuration as valid", output)
	}
}
