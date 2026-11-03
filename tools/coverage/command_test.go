package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCommandHelpAndValidation(t *testing.T) {
	t.Setenv("AHH_COVERAGE_MINIMUM", "invalid")
	for _, tt := range []struct {
		name      string
		args      []string
		wantHelp  string
		wantError bool
	}{
		{name: "help", args: []string{"--help"}, wantHelp: "AHH_COVERAGE_MINIMUM"},
		{name: "unknown command", args: []string{"unknown"}, wantError: true},
		{name: "unknown flag", args: []string{"--unknown"}, wantError: true},
		{name: "root threshold cannot be overridden", args: []string{"--minimum", "0"}, wantError: true},
		{name: "removed report subcommand", args: []string{"report"}, wantError: true},
		{name: "invalid environment", args: []string{}, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newCommand()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(tt.args)
			err := cmd.ExecuteContext(t.Context())
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v", err)
			}
			if tt.wantHelp != "" && !strings.Contains(output.String(), tt.wantHelp) {
				t.Fatal(output.String())
			}
			if tt.name == "invalid environment" && !strings.Contains(err.Error(), "AHH_COVERAGE_MINIMUM") {
				t.Fatalf("expected environment validation error, got %v", err)
			}
			if tt.wantError && output.Len() != 0 {
				t.Fatalf("unexpected usage or duplicate error: %s", &output)
			}
		})
	}
}

func TestRootCommandPropagatesCancellation(t *testing.T) {
	t.Setenv("AHH_COVERAGE_MINIMUM", "90")
	cmd := newCommand()
	cmd.SetArgs([]string{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// The default command must invoke the collector with the supplied context.
	if err := cmd.ExecuteContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestCoverageMinimumEnvironment(t *testing.T) {
	for _, tt := range []struct {
		name      string
		value     string
		unset     bool
		want      int
		wantError bool
	}{
		{name: "unset", unset: true, want: 95},
		{name: "override", value: "90", want: 90},
		{name: "zero", value: "0", want: 0},
		{name: "hundred", value: "100", want: 100},
		{name: "whitespace", value: " 95\n", want: 95},
		{name: "empty", value: "", wantError: true},
		{name: "text", value: "bad", wantError: true},
		{name: "fraction", value: "90.5", wantError: true},
		{name: "negative", value: "-1", wantError: true},
		{name: "above hundred", value: "101", wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AHH_COVERAGE_MINIMUM", tt.value)
			if tt.unset {
				if err := os.Unsetenv("AHH_COVERAGE_MINIMUM"); err != nil {
					t.Fatal(err)
				}
			}
			minimum, err := coverageMinimum()
			if tt.wantError {
				if err == nil || !strings.Contains(err.Error(), "AHH_COVERAGE_MINIMUM") {
					t.Fatalf("expected environment validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if minimum != tt.want {
				t.Fatalf("minimum = %d, want %d", minimum, tt.want)
			}
		})
	}
}
