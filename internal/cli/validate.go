package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"gscacco.com/cronx/internal/config"
)

// newValidateCommand builds the "validate" command, which reports whether the
// configuration file is valid.
func newValidateCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the configuration file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.ResolvePath(*configPath)
			if err != nil {
				return err
			}
			if _, err := config.Load(path); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is valid\n", path)
			return nil
		},
	}
}
