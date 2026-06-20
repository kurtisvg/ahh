package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

type harnessType string

const harnessClaudeCode harnessType = "claude-code"

type runOpts struct {
	harness harnessType
}

// parseRunOpts validates the shared harness argument for ahh run.
func parseRunOpts(opts *runOpts, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected one harness argument")
	}
	switch harness := harnessType(args[0]); harness {
	case harnessClaudeCode:
		opts.harness = harness
	default:
		return fmt.Errorf("unsupported harness %q", args[0])
	}
	return nil
}

func newRunCmd() *cobra.Command {
	opts := runOpts{}
	cmd := &cobra.Command{
		Use:   "run <harness>",
		Short: "Run a harness wrapper process.",
		Args: func(_ *cobra.Command, args []string) error {
			return parseRunOpts(&opts, args)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHarnessWrapper(cmd, opts)
		},
	}
	return cmd
}

func runHarnessWrapper(*cobra.Command, runOpts) error {
	return errors.New("run is not implemented yet")
}
