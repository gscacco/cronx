package cli

import (
	"fmt"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"gscacco.com/cronx/internal/config"
	"gscacco.com/cronx/internal/store"
)

// defaultHistoryLimit is how many runs "history" shows unless told otherwise.
const defaultHistoryLimit = 20

func newHistoryCommand(configPath *string) *cobra.Command {
	var limit int

	command := &cobra.Command{
		Use:   "history [job]",
		Short: "Show the execution history",
		Long: "Show the execution history, newest first.\n\n" +
			"With a job name only that job is shown; otherwise every job is.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}

			path, err := config.ResolvePath(*configPath)
			if err != nil {
				return err
			}

			environment, err := openEnvironment(path, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer func() { _ = environment.close() }()

			runs, err := environment.store.Runs(cmd.Context(), name, limit)
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no runs have been recorded")
				return nil
			}

			table := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(table, "ID\tJOB\tATTEMPT\tSTATUS\tSTARTED\tDURATION\tEXIT")
			for _, run := range runs {
				fmt.Fprintf(table, "%d\t%s\t%d\t%s\t%s\t%s\t%s\n",
					run.ID,
					run.Job,
					run.Attempt,
					run.Status,
					run.StartedAt.Local().Format(timestampLayout),
					run.Duration.Round(time.Millisecond).String(),
					exitCodeText(run))
			}
			return table.Flush()
		},
	}

	command.Flags().IntVar(&limit, "limit", defaultHistoryLimit, "how many runs to show")
	return command
}

// exitCodeText renders the exit code of a run, or a dash when no process ran.
func exitCodeText(run store.Run) string {
	if run.ExitCode == nil {
		return "-"
	}
	return strconv.Itoa(*run.ExitCode)
}
