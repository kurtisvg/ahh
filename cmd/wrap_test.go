package cmd

import (
	"context"
	"os"
	"path/filepath"
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
	tests := []commandTestCase{
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
			wantErrOut: []string{
				"wrapper listening at http://127.0.0.1:",
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
			wantErrOut: []string{
				"wrapper listening at http://127.0.0.1:",
			},
		},
	}

	for _, tt := range tests {
		runCommandTest(t, tt)
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
