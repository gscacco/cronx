package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/cli"
)

const validConfig = `
[jobs.backup]
schedule = "0 3 * * *"
command = "/usr/local/bin/backup"
`

const invalidConfig = `
[jobs.backup]
schedule = "0 3 * * *"
command = "backup"
`

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing configuration file: %v", err)
	}
	return path
}

// runValidate executes "cronx validate --config path" and returns its combined
// output together with the error it produced.
func runValidate(t *testing.T, path string) (string, error) {
	t.Helper()
	cmd := cli.NewRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"validate", "--config", path})
	err := cmd.Execute()
	return output.String(), err
}

func TestValidateCommandAcceptsValidConfiguration(t *testing.T) {
	// SETUP
	path := writeConfig(t, validConfig)

	// EXERCISE
	output, err := runValidate(t, path)

	// VERIFY
	if err != nil {
		t.Fatalf("validate returned an unexpected error: %v", err)
	}
	if !strings.Contains(output, "is valid") {
		t.Errorf("validate output = %q, want it to report that the configuration is valid", output)
	}
}

func TestValidateCommandRejectsInvalidConfiguration(t *testing.T) {
	// SETUP
	path := writeConfig(t, invalidConfig)

	// EXERCISE
	_, err := runValidate(t, path)

	// VERIFY
	if err == nil {
		t.Fatalf("validate succeeded, want an error for an invalid configuration")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("validate error = %q, want it to explain the problem", err.Error())
	}
}
