package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is the current release of cronx, reported by the "version" command.
const version = "0.1.0"

// newVersionCommand builds the "version" command, which prints the current
// release of cronx.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of cronx",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", commandName, version)
			return nil
		},
	}
}
