package cmd

import (
	"strings"
	"testing"
)

func TestWrapCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		args            []string
		wantOutContains []string
		wantErrContains string
	}{
		{
			name: "shows wrap help",
			args: []string{"wrap", "--help"},
			wantOutContains: []string{
				"Run a harness wrapper process",
				"Usage:",
				"ahh wrap <harness>",
				"ahh wrap claude-code",
			},
		},
		{
			name:            "requires a harness argument",
			args:            []string{"wrap"},
			wantErrContains: "accepts 1 arg(s), received 0",
		},
		{
			name:            "rejects unknown harness",
			args:            []string{"wrap", "nope"},
			wantErrContains: `invalid argument "nope" for "ahh wrap"`,
		},
		{
			name:            "reports Claude Code entrypoint as not implemented",
			args:            []string{"wrap", "claude-code"},
			wantErrContains: "claude-code wrapper entrypoint is not implemented yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := executeRootCommand(tt.args...)

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
