package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/kurtisvg/ahh/internal/version"

	"github.com/spf13/cobra"
)

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

	root.AddCommand(
		newServerCommand(ctx, stdout, runners.server),
		newRunWrapperCommand(ctx, stdout, runners.wrapper),
	)
	return root
}
