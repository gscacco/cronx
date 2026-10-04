// Package cli implements the cronx command-line interface.
package cli

import "github.com/spf13/cobra"

// commandName is the name of the program as shown in its help.
const commandName = "cronx"

// Execute runs the cronx command-line interface and returns any error.
func Execute() error {
	return NewRootCommand().Execute()
}

// NewRootCommand builds the root command with every subcommand attached.
func NewRootCommand() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:           commandName,
		Short:         "cronx is a small, reliable, portable and secure local job scheduler",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "",
		"path to the configuration file (default ~/.cronx/config.toml)")

	root.AddCommand(
		newValidateCommand(&configPath),
		newListCommand(&configPath),
		newStatusCommand(&configPath),
		newHistoryCommand(&configPath),
		newRunOnceCommand(&configPath),
		newRunCommand(&configPath),
	)

	return root
}
