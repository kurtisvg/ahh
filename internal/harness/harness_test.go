package harness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeHarnessScript = `#!/bin/sh
case "${AHH_HELPER_HARNESS_MODE}" in
  success)
    printf 'fake stdout\n'
    exit 0
    ;;
  failure)
    exit 7
    ;;
  wait)
    exec cat >/dev/null
    ;;
  *)
    printf 'unknown helper harness mode "%s"\n' "${AHH_HELPER_HARNESS_MODE}" >&2
    exit 2
    ;;
esac
`

func TestHarness(t *testing.T) {
	tests := []struct {
		name            string
		startHarness    func(*testing.T, context.Context) (Harness, error)
		wantErrContains string
	}{
		{
			name: "runs harness to completion",
			startHarness: func(t *testing.T, ctx context.Context) (Harness, error) {
				return fakeHarness(t, ctx, "success")
			},
		},
		{
			name: "reports missing command",
			startHarness: func(t *testing.T, ctx context.Context) (Harness, error) {
				setHarnessCommand(t, "")
				return StartClaudeCode(ctx)
			},
			wantErrContains: "exec: no command",
		},
		{
			name: "reports failing harness",
			startHarness: func(t *testing.T, ctx context.Context) (Harness, error) {
				return fakeHarness(t, ctx, "failure")
			},
			wantErrContains: `harness "`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness, err := tt.startHarness(t, t.Context())
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			t.Cleanup(harness.Close)
			err = harness.Wait(t.Context())

			if tt.wantErrContains == "" && err != nil {
				t.Fatalf("Wait() error = %v", err)
			}
			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatal("Wait() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("Wait() error = %q, want containing %q", err.Error(), tt.wantErrContains)
				}
			}
		})
	}
}

func TestHarnessWaitReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	harness, err := fakeHarness(t, ctx, "wait")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(harness.Close)
	err = harness.Wait(t.Context())

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestHarnessDone(t *testing.T) {
	harness, err := fakeHarness(t, t.Context(), "wait")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if harness.Done() {
		t.Fatal("Done() = true before Wait(), want false")
	}
	harness.Close()
	if err := harness.Wait(t.Context()); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v", err)
	}
	if !harness.Done() {
		t.Fatal("Done() = false after Wait(), want true")
	}
}

// fakeHarness points StartClaudeCode at a temporary executable so tests exercise
// the real subprocess lifecycle without requiring claude-agent-acp to be installed.
func fakeHarness(t *testing.T, ctx context.Context, mode string) (Harness, error) {
	t.Helper()

	setHarnessCommand(t, fakeHarnessCommand(t))
	t.Setenv("AHH_HELPER_HARNESS_MODE", mode)

	return StartClaudeCode(ctx)
}

func setHarnessCommand(t *testing.T, command string) {
	t.Helper()

	original := claudeAgentACPCommand
	claudeAgentACPCommand = command
	t.Cleanup(func() {
		claudeAgentACPCommand = original
	})
}

func fakeHarnessCommand(t *testing.T) string {
	t.Helper()

	command := filepath.Join(t.TempDir(), "fake-harness")
	if err := os.WriteFile(command, []byte(fakeHarnessScript), 0o755); err != nil {
		t.Fatalf("write fake harness command: %v", err)
	}

	return command
}
