package cmd

import (
	"context"
	"testing"
	"time"
)

func TestServeCommand(t *testing.T) {
	tests := []commandTestCase{
		{
			name: "shows serve help",
			args: []string{"serve", "--help"},
			wantOutContains: []string{
				"Run the Ahh server",
				"Usage:",
				"ahh serve",
				"--config",
			},
		},
		{
			name:            "rejects unexpected args",
			args:            []string{"serve", "claude-code"},
			wantErrContains: `unknown command "claude-code" for "ahh serve"`,
		},
		{
			name:    "starts server",
			args:    []string{"serve"},
			timeout: 100 * time.Millisecond,
			wantErr: context.DeadlineExceeded,
			wantErrOut: []string{
				"server listening at http://127.0.0.1:",
			},
		},
		{
			name:    "returns context error when canceled",
			args:    []string{"serve"},
			timeout: 100 * time.Millisecond,
			wantErr: context.DeadlineExceeded,
			wantErrOut: []string{
				"server listening at http://127.0.0.1:",
			},
		},
	}

	for _, tt := range tests {
		runCommandTest(t, tt)
	}
}
