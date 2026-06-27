package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	fakeClaudeSuccessScript = `#!/bin/sh
exit 0
`

	fakeClaudeWaitScript = `#!/bin/sh
sleep 10
`
)

func TestWrapCommand(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		setup           func(*testing.T)
		timeout         time.Duration
		wantErr         error
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
			name: "starts Claude Code PTY",
			args: []string{"wrap", "claude-code"},
			setup: func(t *testing.T) {
				installFakeClaude(t, fakeClaudeSuccessScript)
			},
		},
		{
			name: "reports missing Claude Code command",
			args: []string{"wrap", "claude-code"},
			setup: func(t *testing.T) {
				t.Setenv("PATH", t.TempDir())
			},
			wantErrContains: "executable file not found",
		},
		{
			name: "returns context error when canceled",
			args: []string{"wrap", "claude-code"},
			setup: func(t *testing.T) {
				installFakeClaude(t, fakeClaudeWaitScript)
			},
			timeout: 100 * time.Millisecond,
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}

			ctx := t.Context()
			if tt.timeout != 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			stdout, stderr, err := executeRootCommand(ctx, tt.args...)

			if tt.wantErrContains == "" && tt.wantErr == nil && err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
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

func installFakeClaude(t *testing.T, script string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude command: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
