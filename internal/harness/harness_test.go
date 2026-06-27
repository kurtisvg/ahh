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
    exit 0
    ;;
  failure)
    exit 7
    ;;
  wait)
    sleep 10
    ;;
  *)
    printf 'unknown helper harness mode "%s"\n' "${AHH_HELPER_HARNESS_MODE}" >&2
    exit 2
    ;;
esac
`

func TestHarness(t *testing.T) {
	tests := []struct {
		name                 string
		startHarness         func(*testing.T, context.Context) (Harness, error)
		wantStartErrContains string
		wantWaitErrContains  string
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
				setHarnessCommand(t, filepath.Join(t.TempDir(), "missing-claude"))
				return Start(ctx)
			},
			wantStartErrContains: "no such file or directory",
		},
		{
			name: "reports failing harness",
			startHarness: func(t *testing.T, ctx context.Context) (Harness, error) {
				return fakeHarness(t, ctx, "failure")
			},
			wantWaitErrContains: `pty command "`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness, err := tt.startHarness(t, t.Context())
			if tt.wantStartErrContains != "" {
				if err == nil {
					t.Fatal("Start() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantStartErrContains) {
					t.Fatalf("Start() error = %q, want containing %q", err.Error(), tt.wantStartErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			t.Cleanup(harness.Close)
			err = harness.Wait(t.Context())

			if tt.wantWaitErrContains == "" && err != nil {
				t.Fatalf("Wait() error = %v", err)
			}
			if tt.wantWaitErrContains != "" {
				if err == nil {
					t.Fatal("Wait() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantWaitErrContains) {
					t.Fatalf("Wait() error = %q, want containing %q", err.Error(), tt.wantWaitErrContains)
				}
			}
		})
	}
}

func TestHarnessWaitReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
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

// fakeHarness points Start at a temporary executable so tests exercise
// the real PTY subprocess lifecycle without requiring Claude Code to be installed.
func fakeHarness(t *testing.T, ctx context.Context, mode string) (Harness, error) {
	t.Helper()

	setHarnessCommand(t, fakeHarnessCommand(t))
	t.Setenv("AHH_HELPER_HARNESS_MODE", mode)

	return Start(ctx)
}

func setHarnessCommand(t *testing.T, command string) {
	t.Helper()

	original := claudeCommand
	claudeCommand = command
	t.Cleanup(func() {
		claudeCommand = original
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
