// Package cmd defines the ahh command line interface.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurtisvg/ahh/internal/version"
	"github.com/spf13/cobra"
)

// Execute runs the root ahh command.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := NewCommand().ExecuteContext(ctx); err != nil {
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
	cmd.AddCommand(newWrapCommand(), newServeCommand())
	return cmd
}
