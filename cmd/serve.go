package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurtisvg/ahh/internal/logging"
	"github.com/kurtisvg/ahh/internal/server"
	"github.com/kurtisvg/ahh/internal/version"

	"github.com/spf13/cobra"
)

type serveOpts struct {
	host string
	port string
}

func parseServeOpts(cmd *cobra.Command, opts *serveOpts) {
	cmd.Flags().StringVar(&opts.host, "host", "127.0.0.1", "HTTP listen host")
	cmd.Flags().StringVar(&opts.port, "port", "8080", "HTTP listen port")
}

func newServeCmd() *cobra.Command {
	opts := serveOpts{}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the local Ahh server.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, opts)
		},
	}
	parseServeOpts(cmd, &opts)
	return cmd
}

func runServe(cmd *cobra.Command, opts serveOpts) error {
	ctx := cmd.Context()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd.Printf("Ahh %s -- the agent harness harness\n", version.Version)
	ln, err := server.Listen(opts.host, opts.port)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	cmd.Printf("Listening on http://%s\n", ln.Addr().String())

	if err := server.Serve(ctx, ln); err != nil {
		logging.FromContext(ctx).Error("server error", "error", err)
		return err
	}
	return nil
}
