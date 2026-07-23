package projects

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecRunnerReturnsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := (execRunner{}).Run(ctx, nil, "--version")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
}

func TestExecRunnerDisablesCredentialPrompts(t *testing.T) {
	output, err := (execRunner{}).Run(
		t.Context(),
		[]string{"GIT_TERMINAL_PROMPT=1", "GCM_INTERACTIVE=Always"},
		"-c", "alias.show-environment=!env", "show-environment",
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	env := string(output)
	for _, entry := range []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never"} {
		if !strings.Contains(env, entry) {
			t.Fatalf("environment does not contain %q", entry)
		}
	}
}
