package cmd

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/kurtisvg/ahh/internal/server"
	"github.com/kurtisvg/ahh/internal/store"
	"github.com/spf13/cobra"
)

type serveOpts struct {
	address   string
	port      int
	workspace string
	basePath  string
}

func newServeCommand() *cobra.Command {
	var opts serveOpts
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the local orchestrator",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServeCommand(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.address, "address", "127.0.0.1", "Address to bind")
	cmd.Flags().IntVar(&opts.port, "port", 8080, "Port to bind (0 chooses an available port)")
	cmd.Flags().StringVar(&opts.workspace, "workspace", "./ahh-workspace", "Directory for Project repositories and metadata")
	cmd.Flags().StringVar(&opts.basePath, "base-path", "/", "Public URL path prefix (reverse proxy must strip it)")
	return cmd
}

func runServeCommand(cmd *cobra.Command, opts serveOpts) error {
	ctx := cmd.Context()
	srv, err := server.Start(ctx, server.Config{
		Address:   opts.address,
		Port:      opts.port,
		BasePath:  opts.basePath,
		Workspace: opts.workspace,
		Store: store.Config{
			Backend: store.SQLite,
			Path:    "ahh.db",
		},
	})
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		_ = srv.Wait()
	}()

	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Workspace: %s\n", srv.Workspace); err != nil {
		return err
	}
	address := net.JoinHostPort(srv.Address, strconv.Itoa(srv.Port))
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "http://%s\n", address); err != nil {
		return fmt.Errorf("report server startup: %w", err)
	}

	if err := srv.Wait(); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
