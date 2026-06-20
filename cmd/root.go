package cmd

import (
	"errors"
	"log/slog"
	"os"

	"github.com/kurtisvg/ahh/internal/logging"
	"github.com/kurtisvg/ahh/internal/version"

	"github.com/spf13/cobra"
)

// Execute parses CLI flags and starts the Ahh server.
func Execute() {
	cmd := newRootCommand()
	cmd.SetContext(logging.WithLogger(cmd.Context(), slog.Default()))
	if err := cmd.Execute(); err != nil {
		cmd.PrintErrln(err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "ahh",
		Short:         "ahh - the agent harness harness.",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd.Usage()
			return errors.New("missing command")
		},
	}

	root.AddCommand(newServeCmd())
	root.AddCommand(newRunCmd())
	return root
}
