package cmd

import (
	"context"
	"fmt"
	"io"
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

func newServerCommand(ctx context.Context, stdout io.Writer, run func(context.Context, io.Writer, serverOptions) error) *cobra.Command {
	opts := serverOptions{host: "127.0.0.1", port: "8080"}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the local Ahh server.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(ctx, stdout, opts)
		},
	}
	cmd.Flags().StringVar(&opts.host, "host", opts.host, "HTTP listen host")
	cmd.Flags().StringVar(&opts.port, "port", opts.port, "HTTP listen port")
	return cmd
}

func runServer(ctx context.Context, stdout io.Writer, opts serverOptions) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(stdout, "Ahh %s -- the agent harness harness\n", version.Version)
	ln, err := server.Listen(opts.host, opts.port)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	fmt.Fprintf(stdout, "Listening on http://%s\n", ln.Addr().String())

	if err := server.Serve(ctx, ln); err != nil {
		slog.Error("server error", "error", err)
		return err
	}
	return nil
}
