package process

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	p, err := Start(t.Context(), args)
	if err != nil {
		t.Fatal(err)
	}
	cleanupProcess(t, p)

	if _, err := io.Copy(&output, p); err != nil {
		t.Fatal(err)
	}

	// Reading output and waiting for the process preserve its exit status.
	var exitError *exec.ExitError
	if err := p.Wait(); !errors.As(err, &exitError) || exitError.ExitCode() != 7 {
		t.Fatalf("exit = %v", err)
	}
	if !strings.Contains(output.String(), directory) || !strings.Contains(output.String(), "24 80") {
		t.Fatalf("unexpected terminal output: %q", output.String())
	}
}

func TestProcessLaunchFailure(t *testing.T) {
	if p, err := Start(t.Context(), nil); p != nil || err == nil {
		t.Fatalf("empty command: process = %v, error = %v", p, err)
	}

	if _, err := Start(t.Context(), []string{"/nonexistent/harness"}); err == nil {
		t.Fatal("missing executable started")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	args := []string{"sh", fixturePath(t)}
	_, err := Start(ctx, args)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
}

func TestProcessInputAndResize(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	var output bytes.Buffer
	args := []string{"sh", fixturePath(t)}
	p, err := Start(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	cleanupProcess(t, p)

	// Input must remain writable after the resize ioctl.
	if err := p.Resize(31, 92); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(p, "hello\nsize\nexit\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(&output, p); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"received: hello", "31 92"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q: %q", want, output.String())
		}
	}
}

func TestProcessOutputRemainsAfterWait(t *testing.T) {
	var output bytes.Buffer
	args := []string{"sh", "-c", "printf 'final output'"}
	p, err := Start(t.Context(), args)
	if err != nil {
		t.Fatal(err)
	}
	cleanupProcess(t, p)

	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(&output, p); err != nil {
		t.Fatal(err)
	}
	if output.String() != "final output" {
		t.Fatalf("final output = %q", output.String())
	}
	if err := p.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.pty.Write([]byte("input")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("PTY not closed after Shutdown: %v", err)
	}
}

func TestProcessShutdownUnblocksRead(t *testing.T) {
	for _, method := range []string{"cancel", "shutdown"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			p, err := Start(ctx, []string{"sh", fixturePath(t)})
			if err != nil {
				t.Fatal(err)
			}
			cleanupProcess(t, p)

			reader := bufio.NewReader(p)
			// Consume the ready message and initial size; no more output is expected.
			for range 2 {
				if _, err := reader.ReadString('\n'); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				_, err := reader.ReadByte()
				done <- err
			}()

			if method == "cancel" {
				cancel()
			} else if err := p.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, io.EOF) {
					t.Fatalf("read after shutdown = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("read remained blocked")
			}
		})
	}
}

func TestProcessConcurrentReadWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	p, err := Start(ctx, []string{"sh", fixturePath(t)})
	if err != nil {
		t.Fatal(err)
	}
	cleanupProcess(t, p)

	var output bytes.Buffer
	readDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&output, p)
		readDone <- err
	}()

	// One caller orders writes and resizes while the reader drains output.
	for i := range 8 {
		if _, err := fmt.Fprintf(p, "line-%02d\n", i); err != nil {
			t.Fatal(err)
		}
		if err := p.Resize(31, 92); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := io.WriteString(p, "exit\n"); err != nil {
		t.Fatal(err)
	}
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	for i := range 8 {
		want := fmt.Sprintf("received: line-%02d\r\n", i)
		if strings.Count(output.String(), want) != 1 {
			t.Fatalf("expected one %q in output: %q", want, output.String())
		}
	}
}

func TestProcessShutdownUnblocksWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	// Disable echo and leave stdin unread so terminal input eventually blocks.
	args := []string{"sh", "-c", "stty raw -echo; printf 'ready\n'; sleep 60"}
	p, err := Start(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	cleanupProcess(t, p)

	reader := bufio.NewReader(p)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, reader)
		readDone <- err
	}()

	started := make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		close(started)
		_, err := p.Write(make([]byte, 1<<20))
		writeDone <- err
	}()
	<-started
	select {
	case err := <-writeDone:
		t.Fatalf("write finished before shutdown: %v", err)
	case <-time.After(20 * time.Millisecond):
		// The input queue is full; shutdown must interrupt the pending write.
	}
	if err := p.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-writeDone:
		if err == nil {
			t.Fatal("input unexpectedly completed without a consumer")
		}
	case <-ctx.Done():
		t.Fatal("write remained blocked after shutdown")
	}
	select {
	case err := <-readDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("read remained blocked after shutdown")
	}
}

func TestProcessShutdown(t *testing.T) {
	for _, method := range []string{"cancel", "shutdown"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			args := []string{"sh", fixturePath(t)}
			p, err := Start(ctx, args)
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
			p, err := Start(ctx, args)
			if err != nil {
				t.Fatal(err)
			}
			cleanupProcess(t, p)
			readDone := make(chan struct{})
			go func() {
				defer close(readDone)
				_, _ = io.Copy(output, p)
			}()
			defer func() {
				_ = p.Shutdown(t.Context())
				<-readDone
			}()

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
