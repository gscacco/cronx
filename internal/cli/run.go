package cli

import (
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/logx"
)

func newRunCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the scheduler until it is stopped",
		Long: "Run the scheduler in the foreground until it is stopped.\n\n" +
			"Jobs run as their schedule says. Interrupting the process stops the\n" +
			"jobs that are still running and waits for them before exiting.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.ResolvePath(*configPath)
			if err != nil {
				return err
			}

			// The scheduler writes to its own log file and, when someone is
			// watching, to the standard error as well.
			logs, err := logx.DefaultLayout()
			if err != nil {
				return err
			}
			logFile, err := logs.OpenSchedulerLog()
			if err != nil {
				return err
			}
			defer func() { _ = logFile.Close() }()

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			environment, err := openEnvironment(path, io.MultiWriter(logFile, cmd.ErrOrStderr()))
			if err != nil {
				return err
			}
			defer func() { _ = environment.close() }()

			// A configuration change does not need a restart: the scheduler
			// reloads the file whenever it is sent SIGHUP.
			go watchForReload(ctx, path, environment.scheduler, environment.logger)

			return environment.scheduler.Run(ctx)
		},
	}
}
