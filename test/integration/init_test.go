package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWritesAConfigurationForAFreshInstallation(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)

	// EXERCISE
	written := environment.runOK("init")

	// VERIFY: the file is where the default path points, it is private, and it
	// is all the command wrote: no state was opened and no log was created.
	path := environment.defaultConfigPath()
	if !strings.Contains(written.stdout, path) {
		t.Errorf("init printed %q, want it to name %s", written.stdout, path)
	}
	if mode := permissionMode(t, path); mode != 0o600 {
		t.Errorf("the generated configuration is %v, want 0600", mode)
	}
	if mode := permissionMode(t, filepath.Dir(path)); mode != 0o700 {
		t.Errorf("the generated directory is %v, want 0700", mode)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading the configuration directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.toml" {
		t.Errorf("the configuration directory holds %v, want only the file init wrote\n%s",
			entries, environment.describe())
	}

	// VERIFY: the other commands read it as the configuration of this
	// installation, without being told which file to use.
	if validated := environment.runOK("validate"); !strings.Contains(validated.stdout, "is valid") {
		t.Errorf("validate printed %q, want it to report the generated file as valid", validated.stdout)
	}
	listed := environment.runOK("list")
	for _, want := range []string{"hello", "*/5 * * * *"} {
		if !strings.Contains(listed.stdout, want) {
			t.Errorf("list printed %q, want it to contain %q", listed.stdout, want)
		}
	}
}

func TestInitRefusesToReplaceAConfigurationThatIsAlreadyThere(t *testing.T) {
	// SETUP: an installation whose configuration is already written and in use.
	environment := newEnvironment(t)
	configPath := environment.configureAt("heartbeat.toml", environment.defaultConfigPath())
	before := environment.readFile(configPath)

	// EXERCISE
	refused := environment.run("init")

	// VERIFY: the file is named, the way out is named with it, and the file
	// itself is left as it was.
	if refused.code == 0 {
		t.Fatalf("init exited with status 0, want it to refuse to replace a configuration that exists")
	}
	for _, want := range []string{configPath, "--force"} {
		if !strings.Contains(refused.stderr, want) {
			t.Errorf("init printed %q on standard error, want it to contain %q",
				refused.stderr, want)
		}
	}
	if after := environment.readFile(configPath); after != before {
		t.Errorf("the configuration is now %q, want it left as it was:\n%s", after, before)
	}

	// EXERCISE: asked to, the command replaces the file.
	environment.runOK("init", "--force")

	// VERIFY: what is there is the file init writes, and cronx accepts it.
	after := environment.readFile(configPath)
	if after == before {
		t.Error("the configuration was not replaced")
	}
	if strings.Contains(after, "heartbeat") {
		t.Errorf("the configuration still holds the job that was there:\n%s", after)
	}
	if validated := environment.runOK("validate"); !strings.Contains(validated.stdout, "is valid") {
		t.Errorf("validate printed %q on the replaced configuration, want it to be valid", validated.stdout)
	}
}

func TestInitWritesWhereTheConfigurationIsAskedFor(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	elsewhere := environment.path(filepath.Join("nested", "jobs.toml"))
	fromVariable := environment.path("from-variable.toml")

	// EXERCISE: --config names the file, in a directory that does not exist yet.
	byFlag := environment.runOK("init", "--config", elsewhere)

	// EXERCISE: CRONX_CONFIG names it when no flag does.
	byVariable := environment.runWithEnvironment([]string{"CRONX_CONFIG=" + fromVariable}, "init")

	// VERIFY
	for _, written := range []struct {
		description string
		outcome     result
		path        string
	}{
		{description: "--config", outcome: byFlag, path: elsewhere},
		{description: "CRONX_CONFIG", outcome: byVariable, path: fromVariable},
	} {
		if written.outcome.code != 0 {
			t.Errorf("init with %s exited with status %d, want success\nstderr:\n%s",
				written.description, written.outcome.code, written.outcome.stderr)
			continue
		}
		if !strings.Contains(written.outcome.stdout, written.path) {
			t.Errorf("init with %s printed %q, want it to name %s",
				written.description, written.outcome.stdout, written.path)
		}
		validated := environment.run("validate", "--config", written.path)
		if validated.code != 0 || !strings.Contains(validated.stdout, "is valid") {
			t.Errorf("validate on the file init wrote with %s printed %q on standard output and %q on standard error, want it to be valid",
				written.description, validated.stdout, validated.stderr)
		}
	}
	if mode := permissionMode(t, filepath.Dir(elsewhere)); mode != 0o700 {
		t.Errorf("the directory init created is %v, want 0700", mode)
	}
	if _, err := os.Stat(environment.defaultConfigPath()); err == nil {
		t.Errorf("the default configuration %s was written, want only the file that was asked for",
			environment.defaultConfigPath())
	}
}
