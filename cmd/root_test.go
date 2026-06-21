package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/version"
)

func TestRootCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		args            []string
		wantOut         string
		wantOutContains []string
		wantErrContains string
	}{
		{
			name: "shows help with no args",
			wantOutContains: []string{
				"ahh - the agent harness harness",
				"Usage:\n  ahh [flags]",
				"wrap",
			},
		},
		{
			name: "shows help flag",
			args: []string{"--help"},
			wantOutContains: []string{
				"ahh - the agent harness harness",
				"Usage:\n  ahh [flags]",
				"wrap",
			},
		},
		{
			name:    "shows built-in version flag",
			args:    []string{"--version"},
			wantOut: "ahh version " + version.Version + "\n",
		},
		{
			name:            "rejects unexpected positional args",
			args:            []string{"nope"},
			wantErrContains: `unknown command "nope" for "ahh"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := executeRootCommand(t.Context(), tt.args...)

			if tt.wantErrContains == "" && err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatal("Execute() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErrContains)
				}
			}

			if tt.wantOut != "" && stdout != tt.wantOut {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantOut)
			}
			for _, want := range tt.wantOutContains {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout = %q, want containing %q", stdout, want)
				}
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestRootCommandConfiguresCobraErrorHandling(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()

	if !cmd.SilenceUsage {
		t.Error("SilenceUsage = false, want true")
	}
	if !cmd.SilenceErrors {
		t.Error("SilenceErrors = false, want true")
	}
}

func executeRootCommand(ctx context.Context, args ...string) (string, string, error) {
	cmd := NewCommand()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetContext(ctx)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}
