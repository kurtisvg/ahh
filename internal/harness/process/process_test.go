package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessDirectoryAndExit(t *testing.T) {
	var output bytes.Buffer
	fixture := fixturePath(t)
	directory := t.TempDir()
	t.Chdir(directory)

	args := []string{"sh", fixture, "exit"}
	p, err := Start(t.Context(), &output, args)
	if err != nil {
		t.Fatal(err)
	}
	cleanupProcess(t, p)

	// Wait includes draining final output and preserves the child's exit status.
	var exitError *exec.ExitError
	if err := p.Wait(); !errors.As(err, &exitError) || exitError.ExitCode() != 7 {
		t.Fatalf("exit = %v", err)
	}
	if !strings.Contains(output.String(), directory) || !strings.Contains(output.String(), "24 80") {
		t.Fatalf("unexpected terminal output: %q", output.String())
	}
}

func TestProcessLaunchFailure(t *testing.T) {
	if p, err := Start(t.Context(), io.Discard, nil); p != nil || err == nil {
		t.Fatalf("empty command: process = %v, error = %v", p, err)
	}

	if _, err := Start(t.Context(), io.Discard, []string{"/nonexistent/harness"}); err == nil {
		t.Fatal("missing executable started")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	args := []string{"sh", fixturePath(t)}
	_, err := Start(ctx, io.Discard, args)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
}

func TestProcessShutdown(t *testing.T) {
	for _, method := range []string{"cancel", "shutdown"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			args := []string{"sh", fixturePath(t)}
			p, err := Start(ctx, io.Discard, args)
			if err != nil {
				t.Fatal(err)
			}
			cleanupProcess(t, p)

			if method == "cancel" {
				cancel()
			} else if err := p.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
			_ = p.Wait()

			// Cleanup remains safe after the process and PTY reader have stopped.
			if err := p.Shutdown(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProcessForcedShutdown(t *testing.T) {
	for _, method := range []string{"cancel", "deadline"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			output := &readyWriter{ready: make(chan struct{})}
			args := []string{"sh", fixturePath(t), "ignore-term"}
			p, err := Start(ctx, output, args)
			if err != nil {
				t.Fatal(err)
			}
			cleanupProcess(t, p)

			// The fixture installs its signal handler before producing output.
			select {
			case <-output.ready:
			case <-time.After(3 * time.Second):
				t.Fatal("fixture did not become ready")
			}

			if method == "cancel" {
				cancel()
			} else {
				shutdown, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				defer stop()
				if err := p.Shutdown(shutdown); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("shutdown error = %v", err)
				}
			}

			var exitError *exec.ExitError
			if err := p.Wait(); !errors.As(err, &exitError) || exitError.ExitCode() != -1 {
				t.Fatalf("expected forced termination, got %v", err)
			}
		})
	}
}

func cleanupProcess(t *testing.T, p *Process) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = p.Shutdown(ctx)
		_ = p.Wait()
	})
}

func fixturePath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("testdata/fake-harness")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

type readyWriter struct {
	ready chan struct{}
}

func (w *readyWriter) Write(data []byte) (int, error) {
	select {
	case <-w.ready:
	default:
		close(w.ready)
	}
	return len(data), nil
}
