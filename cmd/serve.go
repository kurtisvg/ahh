package cmd

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kurtisvg/ahh/internal/project/local"
	"github.com/kurtisvg/ahh/internal/server"
	"github.com/kurtisvg/ahh/internal/store"
	"github.com/spf13/cobra"
)

type serveOpts struct {
	address   string
	port      int
	workspace string
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
	return cmd
}

func runServeCommand(cmd *cobra.Command, opts serveOpts) error {
	ctx := cmd.Context()
	workspace, err := local.Open(opts.workspace)
	if err != nil {
		return fmt.Errorf("open workspace: %w", err)
	}
	defer func() { _ = workspace.Close() }()
	srv, err := server.Start(ctx, server.Config{
		Address: opts.address,
		Port:    opts.port,
		Store: store.Config{
			Backend: store.SQLite,
			Path:    filepath.Join(workspace.Path(), "ahh.db"),
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

	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Workspace: %s\n", workspace.Path()); err != nil {
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
