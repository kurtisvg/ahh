//go:build integration

package cmd

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestWrapClaudeCodeStartsRealHarness(t *testing.T) {
	if _, err := exec.LookPath("claude-agent-acp"); err != nil {
		t.Skip("claude-agent-acp is not installed or not on PATH")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	_, _, err := executeRootCommand(ctx, "wrap", "claude-code")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v, want %v", err, context.DeadlineExceeded)
	}
}
