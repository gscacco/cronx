package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

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

			out := cmd.OutOrStdout()
			names := environment.jobNames()
			if len(names) == 0 {
				fmt.Fprintln(out, "no jobs are configured")
			} else {
				table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
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
				if err := table.Flush(); err != nil {
					return err
				}
			}

			fmt.Fprintln(out)
			return describeLease(cmd, environment)
		},
	}
}

// describeLease reports whether a scheduler is driving this state, which is what
// tells a reader whether the jobs are being scheduled at all. The lease of a
// scheduler that was killed instead of stopping is reported as what it is: a
// state that is free, left behind by the holder named.
func describeLease(cmd *cobra.Command, environment *environment) error {
	held, found, err := environment.store.CurrentLease(cmd.Context())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	// The instants of a lease are rendered like the instants of a run: on the
	// clock of the machine that reads them.
	moment := func(at time.Time) string { return at.Local().Format(timestampLayout) }

	switch {
	case !found:
		fmt.Fprintln(out, "no scheduler is running on this state")
	case held.ExpiresAt.After(environment.clock.Now()):
		fmt.Fprintf(out, "scheduler running since %s (%s), lease until %s\n",
			moment(held.AcquiredAt), held.Holder, moment(held.ExpiresAt))
	default:
		fmt.Fprintf(out, "no scheduler is running on this state: the lease %s took %s expired at %s\n",
			held.Holder, moment(held.AcquiredAt), moment(held.ExpiresAt))
	}
	return nil
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
