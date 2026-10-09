package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"gscacco.com/cronx/internal/config"
)

// timestampLayout is how cronx renders instants for people.
const timestampLayout = "2006-01-02 15:04:05 MST"

func newListCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the configured jobs and when they run next",
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

			now := environment.clock.Now()
			table := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(table, "JOB\tSCHEDULE\tNEXT")
			for _, name := range names {
				fmt.Fprintf(table, "%s\t%s\t%s\n",
					name,
					environment.configuration.Jobs[name].Schedule,
					nextActivationText(environment, name, now))
			}
			return table.Flush()
		},
	}
}

// nextActivationText renders when a job runs next: an instant, the marker of a
// job that a person disabled, the marker of a job that runs when the scheduler
// starts, or a dash when it never will.
func nextActivationText(environment *environment, name string, now time.Time) string {
	switch {
	case environment.scheduler.Disabled(name):
		return disabledText
	case environment.scheduler.RunsAtStartup(name):
		return startupText
	}
	next, ok := environment.scheduler.Next(name, now)
	if !ok {
		return "-"
	}
	return next.Format(timestampLayout)
}

// startupText is what the NEXT column says of a job that is triggered by the
// scheduler starting rather than by the clock: it is waiting for a scheduler
// that is not running, which is a different thing from a schedule that can
// never match.
const startupText = "at startup"

// disabledText is what the NEXT column says of a job the configuration keeps
// without scheduling: it has no activation at all until it is enabled again.
const disabledText = "disabled"
