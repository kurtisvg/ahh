package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fakeClaudeAgentACPSuccessScript = `#!/bin/sh
exit 0
`

	fakeClaudeAgentACPFailureScript = `#!/bin/sh
exit 7
`
)

func TestWrapCommand(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		setup           func(*testing.T)
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
			name: "starts Claude Code harness",
			args: []string{"wrap", "claude-code"},
			setup: func(t *testing.T) {
				installFakeClaudeAgentACP(t, fakeClaudeAgentACPSuccessScript)
			},
		},
		{
			name: "reports missing Claude Code harness command",
			args: []string{"wrap", "claude-code"},
			setup: func(t *testing.T) {
				t.Setenv("PATH", t.TempDir())
			},
			wantErrContains: "executable file not found",
		},
		{
			name: "reports Claude Code harness runtime failures",
			args: []string{"wrap", "claude-code"},
			setup: func(t *testing.T) {
				installFakeClaudeAgentACP(t, fakeClaudeAgentACPFailureScript)
			},
			wantErrContains: "exit status 7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}

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

func installFakeClaudeAgentACP(t *testing.T, script string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "claude-agent-acp")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude-agent-acp: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
