package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/job"
)

func newRunOnceCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "run-once <job>",
		Short: "Run a job immediately, whatever its schedule",
		Long: "Run a job immediately, whatever its schedule.\n\n" +
			"The run is recorded in the history and its output is written to the\n" +
			"usual log file, exactly as it would be by the scheduler. The overlap,\n" +
			"retry and timeout policies of the job still apply.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			path, err := config.ResolvePath(*configPath)
			if err != nil {
				return err
			}

			environment, err := openEnvironment(path, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer func() { _ = environment.close() }()

			status, err := environment.scheduler.Execute(cmd.Context(), name, environment.clock.Now())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "job %q finished with status %q\n", name, status)

			if status != job.StatusSucceeded {
				return fmt.Errorf("job %q did not succeed", name)
			}
			return nil
		},
	}
}
