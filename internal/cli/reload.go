package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/scheduler"
)

// watchForReload reloads the configuration whenever the process is sent
// SIGHUP, until the scheduler stops.
//
// A configuration that cannot be read or used is refused and reported: the
// scheduler keeps running with the one it has, rather than stopping because the
// file happened to be half written when the signal arrived. A reload applies
// the jobs; a change to a setting of the scheduler itself is reported by the
// scheduler, because it needs a restart.
func watchForReload(ctx context.Context, path string, driver *scheduler.Scheduler, logger *slog.Logger) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP)
	defer signal.Stop(signals)

	for {
		select {
		case <-ctx.Done():
			return
		case <-signals:
		}

		reloaded, err := config.Load(path)
		if err != nil {
			logger.Error("the configuration could not be reloaded, so the running one is kept",
				"error", err)
			continue
		}
		if err := driver.Reload(*reloaded); err != nil {
			logger.Error("the configuration could not be reloaded, so the running one is kept",
				"error", err)
			continue
		}
		logger.Info("configuration reloaded", "path", path)
	}
}
