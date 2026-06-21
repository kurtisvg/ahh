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
			name: "shows namespace help with no args",
			args: []string{"wrap"},
			wantOutContains: []string{
				"Run harness wrapper processes",
				"Usage:",
				"ahh wrap",
			},
		},
		{
			name:            "rejects unknown wrapper command",
			args:            []string{"wrap", "nope"},
			wantErrContains: `unknown command "nope" for "ahh wrap"`,
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
