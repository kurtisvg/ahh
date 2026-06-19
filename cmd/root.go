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

// Execute parses CLI flags and starts the Ahh server.
func Execute() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := newRootCommand()
	cmd.SetContext(ctx)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	if len(args) == 0 {
		cmd.SetOut(stderr)
	}
	return cmd.Execute()
}

func newRootCommand() *cobra.Command {
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

	root.AddCommand(newServeCmd())
	root.AddCommand(newRunWrapperCmd())
	return root
}
