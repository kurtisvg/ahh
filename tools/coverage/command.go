package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const defaultMinimum = 95

func newCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "coverage",
		Short: "Run the full suite and check Go coverage",
		Long: fmt.Sprintf(
			"Run the full suite and check Go coverage.\n\n"+
				"AHH_COVERAGE_MINIMUM sets the minimum (default: %d%%).",
			defaultMinimum,
		),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			minimum, err := coverageMinimum()
			if err != nil {
				return err
			}
			return collect(cmd.Context(), minimum, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

// coverageMinimum uses the default only when AHH_COVERAGE_MINIMUM is unset.
// Empty values and values outside the whole-percentage range 0–100 are errors.
func coverageMinimum() (int, error) {
	value, ok := os.LookupEnv("AHH_COVERAGE_MINIMUM")
	if !ok {
		return defaultMinimum, nil
	}
	minimum, err := strconv.Atoi(strings.TrimSpace(value))
	validMinimum := minimum >= 0 && minimum <= 100
	if err != nil || !validMinimum {
		return 0, errors.New("AHH_COVERAGE_MINIMUM must be a whole percentage between 0 and 100")
	}
	return minimum, nil
}
