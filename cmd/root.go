// Package cmd defines the ahh command line interface.
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
		_, _ = fmt.Fprintln(os.Stderr, "ahh:", err)
		os.Exit(1)
	}
}

// NewCommand returns the root ahh command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ahh",
		Short:         "ahh - the agent harness harness",
		Version:       version.Version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	return cmd
}
