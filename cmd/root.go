package cmd

import (
	"context"
	"errors"
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

type wrapperOptions struct {
	harness string
}

type commandRunners struct {
	server  func(context.Context, io.Writer, serverOptions) error
	wrapper func(context.Context, io.Writer, wrapperOptions) error
}

// Execute parses CLI flags and starts the Ahh server.
func Execute() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return runWithRunners(ctx, args, stdout, stderr, commandRunners{
		server:  runServer,
		wrapper: runWrapper,
	})
}

func runWithRunners(ctx context.Context, args []string, stdout, stderr io.Writer, runners commandRunners) error {
	cmd := newRootCommand(ctx, stdout, stderr, runners)
	cmd.SetArgs(args)
	if len(args) == 0 {
		cmd.SetOut(stderr)
	}
	return cmd.Execute()
}

func newRootCommand(ctx context.Context, stdout, stderr io.Writer, runners commandRunners) *cobra.Command {
	root := &cobra.Command{
		Use:           "ahh",
		Short:         "Ahh is the agent harness harness.",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Usage()
			return errors.New("missing command")
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetVersionTemplate("{{.Version}}\n")

	serverOpts := serverOptions{host: "127.0.0.1", port: "8080"}
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Run the local Ahh server.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runners.server(ctx, stdout, serverOpts)
		},
	}
	serverCmd.Flags().StringVar(&serverOpts.host, "host", serverOpts.host, "HTTP listen host")
	serverCmd.Flags().StringVar(&serverOpts.port, "port", serverOpts.port, "HTTP listen port")

	runWrapperCmd := &cobra.Command{
		Use:   "run-wrapper",
		Short: "Run a harness wrapper process.",
	}
	claudeCodeOpts := wrapperOptions{harness: "claude-code"}
	claudeCodeCmd := &cobra.Command{
		Use:   "claude-code",
		Short: "Run the Claude Code wrapper process.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runners.wrapper(ctx, stdout, claudeCodeOpts)
		},
	}
	runWrapperCmd.AddCommand(claudeCodeCmd)
	root.AddCommand(serverCmd, runWrapperCmd)
	return root
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

func runWrapper(context.Context, io.Writer, wrapperOptions) error {
	return errors.New("run-wrapper is not implemented yet")
}
