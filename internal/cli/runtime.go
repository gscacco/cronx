package cli

import (
	"io"
	"log/slog"
	"sort"

	"gscacco.com/cronx/internal/clock"
	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/logx"
	"gscacco.com/cronx/internal/runner"
	"gscacco.com/cronx/internal/scheduler"
	"gscacco.com/cronx/internal/store"
)

// environment carries what a command needs: the configuration, the store, the
// shared run log and a scheduler built on top of them.
type environment struct {
	configuration config.Config
	store         *store.Store
	logs          *logx.Log
	logger        *slog.Logger
	scheduler     *scheduler.Scheduler
}

// openEnvironment loads the configuration at path and assembles everything the
// commands need. The status database and the run log live where the
// configuration says, or under the home directory when it says nothing. The
// caller must close the returned environment.
func openEnvironment(path string, output io.Writer) (*environment, error) {
	configuration, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	statePath, err := config.StatePath(configuration.Storage.Path)
	if err != nil {
		return nil, err
	}
	logPath, err := config.LogPath(configuration.Logging.Path)
	if err != nil {
		return nil, err
	}

	persistent, err := store.Open(statePath)
	if err != nil {
		return nil, err
	}

	logs := logx.Open(logPath, clock.System{})

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
	err := e.store.Close()
	if logErr := e.logs.Close(); err == nil {
		err = logErr
	}
	return err
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
