package cmd

import (
	"context"
	"fmt"

	ahhserver "github.com/kurtisvg/ahh/internal/server"
	"github.com/spf13/cobra"
)

const defaultServerAddr = "127.0.0.1:0"

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "serve",
		Short:         "Run the Ahh server",
		Example:       "  ahh serve",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          runServeCommand,
	}

	return cmd
}

func runServeCommand(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	server, err := ahhserver.Start(ctx, defaultServerAddr)
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer func() {
		_ = server.Close(context.Background())
	}()

	fmt.Fprintf(cmd.ErrOrStderr(), "server listening at %s\n", server.URL())

	if err := server.Wait(); err != nil {
		return fmt.Errorf("serve server: %w", err)
	}

	return nil
}
