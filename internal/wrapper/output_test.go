package wrapper

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kurtisvg/ahh/internal/harness"
)

func TestOutputFailureStopsHarness(t *testing.T) {
	useFakeCodex(t)

	// Destination errors that resemble PTY closure must still reach the caller.
	for _, writeErr := range []error{errors.New("output failed"), syscall.EIO, os.ErrClosed} {
		t.Run(writeErr.Error(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			h, err := harness.Start(ctx, "codex")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = h.Shutdown(t.Context()) }()

			if err := copyHarnessOutput(ctx, h, failedWriter{err: writeErr}); !errors.Is(err, writeErr) {
				t.Fatalf("output error = %v, want %v", err, writeErr)
			}
			if ctx.Err() != nil {
				t.Fatal("copying failed to stop the harness before the deadline")
			}
			if err := h.Wait(); err == nil {
				t.Fatal("terminated harness lost its exit error")
			}
		})
	}
}

func TestOutputDrainsBeforeCleanup(t *testing.T) {
	useFakeCodex(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	h, err := harness.Start(ctx, "codex")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Shutdown(t.Context()) }()

	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- copyHarnessOutput(ctx, h, &output) }()
	if _, err := io.WriteString(h, "last output\nexit\n"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "received: last output") {
		t.Fatalf("missing final output: %q", output.String())
	}
	if err := h.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Write([]byte("input")); err == nil {
		t.Fatalf("terminal not closed after copying: %v", err)
	}
}

func TestOutputDrainDoesNotWaitForever(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	h := &exitedHarness{
		Reader: reader,
		close:  writer.Close,
	}

	// Model an exited child whose descendant still holds the output stream open.
	done := make(chan error, 1)
	go func() { done <- copyHarnessOutput(t.Context(), h, io.Discard) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		_ = writer.Close()
		<-done
		t.Fatal("output drain did not close the inherited stream")
	}
}

type exitedHarness struct {
	io.Reader
	close func() error
}

func (*exitedHarness) Write(data []byte) (int, error) { return len(data), nil }
func (*exitedHarness) Resize(uint16, uint16) error    { return nil }
func (*exitedHarness) Wait() error                    { return nil }
func (h *exitedHarness) Shutdown(context.Context) error {
	return h.close()
}

type failedWriter struct {
	err error
}

func (w failedWriter) Write([]byte) (int, error) {
	return 0, w.err
}
