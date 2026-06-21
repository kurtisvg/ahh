package cmd

import "github.com/spf13/cobra"

func newWrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "wrap",
		Short:         "Run harness wrapper processes",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}
