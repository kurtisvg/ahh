package main

import (
	"context"
	_ "embed"
	"errors"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed minimum.txt
var minimumText string

func executeCommand(ctx context.Context, args []string) error {
	cmd, err := newCommand()
	if err != nil {
		return err
	}
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

func newCommand() (*cobra.Command, error) {
	minimum, err := strconv.Atoi(strings.TrimSpace(minimumText))
	validMinimum := minimum >= 0 && minimum <= 100
	if err != nil || !validMinimum {
		return nil, errors.New("minimum.txt must contain a whole percentage between 0 and 100")
	}
	cmd := &cobra.Command{
		Use:           "coverage",
		Short:         "Run the full coverage suite and enforce minimum.txt",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return collect(
				cmd.Context(),
				minimum,
				cmd.OutOrStdout(),
				cmd.ErrOrStderr(),
			)
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	cmd.AddCommand(newReportCommand(minimum))
	return cmd, nil
}

func newReportCommand(minimum int) *cobra.Command {
	floor := minimum
	output := "coverage.out"
	report := &cobra.Command{
		Use:   "report PROFILE...",
		Short: "Merge saved profiles and check their coverage",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeReport(
				args,
				output,
				floor,
				cmd.OutOrStdout(),
			)
		},
	}
	report.Flags().IntVar(
		&floor,
		"minimum",
		minimum,
		"minimum whole percentage (0-100)",
	)
	report.Flags().StringVar(
		&output,
		"out",
		output,
		"merged profile",
	)
	return report
}
