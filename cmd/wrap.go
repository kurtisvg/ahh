package cmd

import (
	"context"
	"fmt"

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

	server, err := wrapper.Start(h, defaultWrapperAddr)
	if err != nil {
		return fmt.Errorf("start wrapper server: %w", err)
	}
	defer func() {
		_ = server.Close(context.Background())
	}()

	fmt.Fprintf(cmd.ErrOrStderr(), "wrapper listening at %s\n", server.URL())

	harnessErr := make(chan error, 1)
	go func() {
		harnessErr <- h.Wait(ctx)
	}()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Wait()
	}()

	select {
	case err := <-harnessErr:
		if err != nil {
			return fmt.Errorf("run %s harness: %w", harnessName, err)
		}
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("serve wrapper: %w", err)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
