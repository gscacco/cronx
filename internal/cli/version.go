package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the current release of cronx, reported by the "version" command.
const Version = "0.2.0"

// newVersionCommand builds the "version" command, which prints the current
// release of cronx.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of cronx",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", commandName, Version)
			return nil
		},
	}
}
