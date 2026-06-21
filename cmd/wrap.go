package cmd

import (
	"errors"
	"fmt"

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
		RunE: func(cmd *cobra.Command, args []string) error {
			switch harness := args[0]; harness {
			case "claude-code":
				return errors.New("claude-code wrapper entrypoint is not implemented yet")
			default:
				return fmt.Errorf("unsupported harness %q", harness)
			}
		},
	}

	return cmd
}
