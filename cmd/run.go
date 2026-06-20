package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurtisvg/ahh/internal/logging"
	"github.com/kurtisvg/ahh/internal/wrapper"

	"github.com/spf13/cobra"
)

type harnessType string

const harnessClaudeCode harnessType = "claude-code"

type runOpts struct {
	harness harnessType
	host    string
	port    string
}

// parseRunOpts validates the shared harness argument for ahh run.
func parseRunOpts(opts *runOpts, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected one harness argument")
	}
	switch harness := harnessType(args[0]); harness {
	case harnessClaudeCode:
		opts.harness = harness
	default:
		return fmt.Errorf("unsupported harness %q", args[0])
	}
	return nil
}

func parseRunFlags(cmd *cobra.Command, opts *runOpts) {
	cmd.Flags().StringVar(&opts.host, "host", "127.0.0.1", "HTTP listen host")
	cmd.Flags().StringVar(&opts.port, "port", "18081", "HTTP listen port")
}

func newRunCmd() *cobra.Command {
	opts := runOpts{}
	cmd := &cobra.Command{
		Use:   "run <harness>",
		Short: "Run a harness wrapper process.",
		Args: func(_ *cobra.Command, args []string) error {
			return parseRunOpts(&opts, args)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHarnessWrapper(cmd, opts)
		},
	}
	parseRunFlags(cmd, &opts)
	return cmd
}

func runHarnessWrapper(cmd *cobra.Command, opts runOpts) error {
	ctx := cmd.Context()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := wrapper.Listen(opts.host, opts.port)
	if err != nil {
		return fmt.Errorf("listen wrapper: %w", err)
	}
	cmd.Printf("Running %s wrapper on http://%s\n", opts.harness, ln.Addr().String())

	if err := wrapper.Serve(ctx, ln, string(opts.harness)); err != nil {
		logging.FromContext(ctx).Error("wrapper server error", "error", err)
		return err
	}
	return nil
}
