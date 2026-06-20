package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

type wrapperOptions struct {
	harness string
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a harness wrapper process.",
	}
	claudeCodeOpts := wrapperOptions{harness: "claude-code"}
	claudeCodeCmd := &cobra.Command{
		Use:   "claude-code",
		Short: "Run the Claude Code wrapper process.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHarnessWrapper(cmd, claudeCodeOpts)
		},
	}
	cmd.AddCommand(claudeCodeCmd)
	return cmd
}

func runHarnessWrapper(*cobra.Command, wrapperOptions) error {
	return errors.New("run is not implemented yet")
}
