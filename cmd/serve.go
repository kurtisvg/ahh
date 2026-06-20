package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurtisvg/ahh/internal/server"
	"github.com/kurtisvg/ahh/internal/version"

	"github.com/spf13/cobra"
)

type serverOptions struct {
	host string
	port string
}

func newServeCmd() *cobra.Command {
	opts := serverOptions{host: "127.0.0.1", port: "8080"}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the local Ahh server.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.host, "host", opts.host, "HTTP listen host")
	cmd.Flags().StringVar(&opts.port, "port", opts.port, "HTTP listen port")
	return cmd
}

func runServer(cmd *cobra.Command, opts serverOptions) error {
	ctx := cmd.Context()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Ahh %s -- the agent harness harness\n", version.Version); err != nil {
		return fmt.Errorf("write startup message: %w", err)
	}
	ln, err := server.Listen(opts.host, opts.port)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Listening on http://%s\n", ln.Addr().String()); err != nil {
		return fmt.Errorf("write listen message: %w", err)
	}

	if err := server.Serve(ctx, ln); err != nil {
		slog.Error("server error", "error", err)
		return err
	}
	return nil
}
