package integration_test

import (
	"strings"
	"syscall"
	"testing"
)

// The reload tests check that a configuration change does not need a restart:
// the scheduler reads the file again when it is sent SIGHUP, and keeps running
// with what it has when the file cannot be used.

// sighup asks the scheduler to read its configuration again, the way a service
// manager or an operator does.
func (p *process) sighup() {
	p.t.Helper()
	if err := p.command.Process.Signal(syscall.SIGHUP); err != nil {
		p.t.Fatalf("asking cronx to reload: %v", err)
	}
}

// schedulerLogHolds reports whether the scheduler has recorded the text.
func (e *environment) schedulerLogHolds(text string) bool {
	e.t.Helper()
	return strings.Contains(e.readFile(e.schedulerLog()), text)
}

func TestTheConfigurationIsReloadedOnSighup(t *testing.T) {
	// SETUP: a scheduler running a configuration with one job.
	environment := newEnvironment(t)
	configPath := environment.configure("reload.toml")
	running := environment.startScheduler(configPath)

	// EXERCISE: the file gains a job, and the scheduler is asked to reload.
	environment.configureAt("reload-more.toml", configPath)
	running.sighup()

	// VERIFY: the reload happened, the job it added is named, and the
	// scheduler is still the one driving the state.
	environment.waitFor("the configuration to be reloaded", settleBudget, func() bool {
		return environment.schedulerLogHolds("configuration reloaded") &&
			environment.schedulerLogHolds("jobs added by the reload")
	})
	if !environment.schedulerLogHolds("second") {
		t.Errorf("the scheduler did not name the job the reload added\n%s",
			lastLines(environment.readFile(environment.schedulerLog()), 20))
	}
	if !running.running() {
		t.Fatalf("the scheduler stopped when it was asked to reload\nstderr:\n%s", running.stderr())
	}
	running.stop()
}

func TestAConfigurationThatCannotBeReloadedIsRefused(t *testing.T) {
	// SETUP: a scheduler running a configuration with one job.
	environment := newEnvironment(t)
	configPath := environment.configure("reload.toml")
	running := environment.startScheduler(configPath)

	// EXERCISE: the file is replaced by one that cannot be used, and the
	// scheduler is asked to reload.
	environment.configureAt("reload-broken.toml", configPath)
	running.sighup()

	// VERIFY: the reload is refused, the reason is recorded, and the
	// scheduler keeps running.
	environment.waitFor("the reload to be refused", settleBudget, func() bool {
		return environment.schedulerLogHolds("could not be reloaded")
	})
	if !running.running() {
		t.Fatalf("the scheduler stopped on a configuration that cannot be reloaded\nstderr:\n%s",
			running.stderr())
	}

	// EXERCISE: the file is repaired, and the scheduler is asked again.
	environment.configureAt("reload-more.toml", configPath)
	running.sighup()

	// VERIFY: the reload that works is applied after the one that failed.
	environment.waitFor("the configuration to be reloaded", settleBudget, func() bool {
		return environment.schedulerLogHolds("configuration reloaded")
	})
	running.stop()
}
