package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gscacco.com/cronx/internal/config"
)

func newStatusCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the state of every job",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.ResolvePath(*configPath)
			if err != nil {
				return err
			}

			environment, err := openEnvironment(path, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer func() { _ = environment.close() }()

			names := environment.jobNames()
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no jobs are configured")
				return nil
			}

			table := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(table, "JOB\tLAST STATUS\tLAST RUN\tIN PROGRESS")
			for _, name := range names {
				status, lastRun, err := lastOutcome(cmd, environment, name)
				if err != nil {
					return err
				}
				running, err := environment.store.RunningRuns(cmd.Context(), name)
				if err != nil {
					return err
				}
				fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", name, status, lastRun, yesNo(running > 0))
			}
			return table.Flush()
		},
	}
}

// lastOutcome describes the most recent run of a job.
func lastOutcome(cmd *cobra.Command, environment *environment, name string) (status, startedAt string, err error) {
	runs, err := environment.store.Runs(cmd.Context(), name, 1)
	if err != nil {
		return "", "", err
	}
	if len(runs) == 0 {
		return "never run", "-", nil
	}
	return string(runs[0].Status), runs[0].StartedAt.Local().Format(timestampLayout), nil
}

// yesNo renders a boolean for a table.
func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
