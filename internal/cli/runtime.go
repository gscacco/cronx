package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/logx"
	"gscacco.com/cronx/internal/runner"
	"gscacco.com/cronx/internal/scheduler"
	"gscacco.com/cronx/internal/store"
)

// homeDirectory is the directory, relative to the user's home, holding the
// configuration, the state and the logs.
const homeDirectory = ".cronx"

// stateFileName is the name of the database inside the home directory.
const stateFileName = "state.db"

// environment carries what a command needs: the configuration, the store, the
// log layout and a scheduler built on top of them.
type environment struct {
	configuration config.Config
	store         *store.Store
	logs          logx.Layout
	logger        *slog.Logger
	scheduler     *scheduler.Scheduler
}

// openEnvironment loads the configuration at path and assembles everything the
// commands need. The caller must close the returned environment.
func openEnvironment(path string, output io.Writer) (*environment, error) {
	configuration, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	statePath, logs, err := defaultPaths()
	if err != nil {
		return nil, err
	}

	persistent, err := store.Open(statePath)
	if err != nil {
		return nil, err
	}

	logger, err := logx.NewLogger(output, configuration.Logging.Level)
	if err != nil {
		_ = persistent.Close()
		return nil, err
	}

	built, err := scheduler.New(scheduler.Options{
		Config: *configuration,
		Store:  persistent,
		Logs:   logs,
		Runner: runner.New(clock.System{}),
		Clock:  clock.System{},
		Logger: logger,
	})
	if err != nil {
		_ = persistent.Close()
		return nil, err
	}

	return &environment{
		configuration: *configuration,
		store:         persistent,
		logs:          logs,
		logger:        logger,
		scheduler:     built,
	}, nil
}

// close releases the resources the environment holds.
func (e *environment) close() error {
	return e.store.Close()
}

// jobNames returns the configured job names in ascending order.
func (e *environment) jobNames() []string {
	names := make([]string, 0, len(e.configuration.Jobs))
	for name := range e.configuration.Jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// defaultPaths returns the default location of the state and of the logs.
func defaultPaths() (string, logx.Layout, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", logx.Layout{}, fmt.Errorf("determining the user home directory: %w", err)
	}
	base := filepath.Join(home, homeDirectory)
	return filepath.Join(base, stateFileName), logx.NewLayout(filepath.Join(base, "logs")), nil
}

// defaultLogLayout returns the default log layout.
func defaultLogLayout() (logx.Layout, error) {
	_, logs, err := defaultPaths()
	return logs, err
}
