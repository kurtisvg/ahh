package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandHelpAndValidation(t *testing.T) {
	t.Setenv("AHH_COVERAGE_MINIMUM", "90")
	for _, tt := range []struct {
		name      string
		args      []string
		wantHelp  string
		wantError bool
	}{
		{name: "help", args: []string{"--help"}, wantHelp: "report"},
		{name: "report help", args: []string{"report", "--help"}, wantHelp: "--minimum"},
		{name: "unknown command", args: []string{"unknown"}, wantError: true},
		{name: "unknown flag", args: []string{"--unknown"}, wantError: true},
		{name: "root threshold cannot be overridden", args: []string{"--minimum", "0"}, wantError: true},
		{name: "missing profiles", args: []string{"report"}, wantError: true},
		{name: "invalid number", args: []string{"report", "--minimum", "bad", "missing.out"}, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := newCommand()
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs(tt.args)
			err = cmd.ExecuteContext(t.Context())
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v", err)
			}
			if tt.wantHelp != "" && !strings.Contains(output.String(), tt.wantHelp) {
				t.Fatal(output.String())
			}
			if tt.wantError && output.Len() != 0 {
				t.Fatalf("unexpected usage or duplicate error: %s", &output)
			}
		})
	}
}

func TestReportCommandFlagsAndDefault(t *testing.T) {
	t.Setenv("AHH_COVERAGE_MINIMUM", "90")
	for _, tt := range []struct {
		name      string
		flags     []string
		wantError bool
	}{
		{name: "override", flags: []string{"--minimum", "50"}},
		{name: "configured default", wantError: true},
		{name: "out of range", flags: []string{"--minimum", "101"}, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "input.out"), filepath.Join(dir, "merged.out")
			data := "mode: atomic\n" + modulePath + "/a.go:1.1,2.1 1 1\n" + modulePath + "/a.go:3.1,4.1 1 0\n"
			if err := os.WriteFile(input, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd, err := newCommand()
			if err != nil {
				t.Fatal(err)
			}
			var summary bytes.Buffer
			cmd.SetOut(&summary)
			// Flags following positional arguments should work, too.
			args := append([]string{"report", input, "--out", output}, tt.flags...)
			cmd.SetArgs(args)
			err = cmd.ExecuteContext(t.Context())
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v", err)
			}
			defaultMinimum := "minimum: **90%**"
			if tt.name == "configured default" && !strings.Contains(summary.String(), defaultMinimum) {
				t.Fatalf("ignored configured minimum: %s", &summary)
			}
			if tt.name != "out of range" {
				if _, err := mergeProfiles([]string{output}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRootCommandPropagatesCancellation(t *testing.T) {
	t.Setenv("AHH_COVERAGE_MINIMUM", "90")
	cmd, err := newCommand()
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetArgs([]string{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// The default command must invoke the collector with the supplied context.
	if err := cmd.ExecuteContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandMinimumEnvironment(t *testing.T) {
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
			cmd, err := newCommand()
			if tt.wantError {
				if err == nil || !strings.Contains(err.Error(), "AHH_COVERAGE_MINIMUM") {
					t.Fatalf("expected environment validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			report, _, err := cmd.Find([]string{"report"})
			if err != nil {
				t.Fatal(err)
			}
			minimum, err := report.Flags().GetInt("minimum")
			if err != nil {
				t.Fatal(err)
			}
			if minimum != tt.want {
				t.Fatalf("minimum = %d, want %d", minimum, tt.want)
			}
		})
	}
}
