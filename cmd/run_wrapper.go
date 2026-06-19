package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

type wrapperOptions struct {
	harness string
}

func newRunWrapperCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run-wrapper",
		Short: "Run a harness wrapper process.",
	}
	claudeCodeOpts := wrapperOptions{harness: "claude-code"}
	claudeCodeCmd := &cobra.Command{
		Use:   "claude-code",
		Short: "Run the Claude Code wrapper process.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWrapper(cmd, claudeCodeOpts)
		},
	}
	cmd.AddCommand(claudeCodeCmd)
	return cmd
}

func runWrapper(*cobra.Command, wrapperOptions) error {
	return errors.New("run-wrapper is not implemented yet")
}
