package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/kurtisvg/ahh/internal/version"

	"github.com/spf13/cobra"
)

// Execute parses CLI flags and starts the Ahh server.
func Execute() {
	cmd := newRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
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
	root.AddCommand(newRunWrapperCmd())
	return root
}
