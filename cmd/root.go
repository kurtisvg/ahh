package cmd

import (
	"fmt"
	"os"

	"github.com/kurtisvg/ahh/internal/version"
	"github.com/spf13/cobra"
)

// Execute runs the root ahh command.
func Execute() {
	if err := NewCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// NewCommand returns the root ahh command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ahh",
		Short:         "ahh - the agent harness harness",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newWrapCommand())

	return cmd
}
