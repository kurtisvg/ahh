package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

type harnessType string

const harnessClaudeCode harnessType = "claude-code"

type runOpts struct {
	harness harnessType
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a harness wrapper process.",
	}
	claudeCodeOpts := runOpts{}
	claudeCodeCmd := &cobra.Command{
		Use:   "claude-code",
		Short: "Run the Claude Code wrapper process.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHarnessWrapper(cmd, claudeCodeOpts)
		},
	}
	parseRunOpts(claudeCodeCmd, &claudeCodeOpts, harnessClaudeCode)
	cmd.AddCommand(claudeCodeCmd)
	return cmd
}

func parseRunOpts(_ *cobra.Command, opts *runOpts, harness harnessType) {
	opts.harness = harness
}

func runHarnessWrapper(*cobra.Command, runOpts) error {
	return errors.New("run is not implemented yet")
}
