package cmd

import (
	"context"
	"errors"
	"io"

	"github.com/spf13/cobra"
)

type wrapperOptions struct {
	harness string
}

func newRunWrapperCommand(ctx context.Context, stdout io.Writer, run func(context.Context, io.Writer, wrapperOptions) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run-wrapper",
		Short: "Run a harness wrapper process.",
	}
	claudeCodeOpts := wrapperOptions{harness: "claude-code"}
	claudeCodeCmd := &cobra.Command{
		Use:   "claude-code",
		Short: "Run the Claude Code wrapper process.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(ctx, stdout, claudeCodeOpts)
		},
	}
	cmd.AddCommand(claudeCodeCmd)
	return cmd
}

func runWrapper(context.Context, io.Writer, wrapperOptions) error {
	return errors.New("run-wrapper is not implemented yet")
}
