package cmd

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/harness"
	"github.com/kurtisvg/ahh/internal/wrapper"
	"github.com/spf13/cobra"
)

const defaultWrapperAddr = "127.0.0.1:0"

func newWrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "wrap <harness>",
		Short:         "Run a harness wrapper process",
		Example:       "  ahh wrap claude-code",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:     []string{string(harness.ClaudeCode)},
		RunE:          runWrapCommand,
	}

	return cmd
}

func runWrapCommand(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	harnessType := harness.Type(args[0])

	server, err := wrapper.Start(ctx, harnessType, defaultWrapperAddr, uuid.NewString())
	if err != nil {
		return fmt.Errorf("start wrapper server: %w", err)
	}
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	fmt.Fprintf(cmd.ErrOrStderr(), "wrapper listening at http://%s\n", server.Addr)

	if err := server.Wait(); err != nil {
		return fmt.Errorf("serve wrapper: %w", err)
	}

	return nil
}
