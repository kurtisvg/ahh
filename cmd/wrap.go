package cmd

import (
	"fmt"

	"github.com/kurtisvg/ahh/internal/harness"
	"github.com/spf13/cobra"
)

func newWrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "wrap <harness>",
		Short:         "Run a harness wrapper process",
		Example:       "  ahh wrap claude-code",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:     []string{"claude-code"},
		RunE:          runWrapCommand,
	}

	return cmd
}

func runWrapCommand(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	harnessName := args[0]
	var h harness.Harness
	var err error

	switch harnessName {
	case "claude-code":
		h, err = harness.Start(ctx)
	default:
		return fmt.Errorf("unsupported harness %q", harnessName)
	}
	if err != nil {
		return fmt.Errorf("start %s harness: %w", harnessName, err)
	}
	defer h.Close()

	if err := h.Wait(ctx); err != nil {
		return fmt.Errorf("run %s harness: %w", harnessName, err)
	}

	return nil
}
