// Package process manages the process and terminal lifecycle of a local harness.
package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Process owns one PTY, child process group, and output reader. Create one with Start.
type Process struct {
	cmd *exec.Cmd
	pty *os.File

	done   chan struct{}
	cancel context.CancelFunc
	err    error // Read only after done closes.
}

// Start runs args[0] with args[1:] in a PTY and copies its output to output, which must not block.
// Cancel ctx or call Shutdown to stop the process group; call Wait to join cleanup.
func Start(ctx context.Context, output io.Writer, args []string) (*Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("missing harness command")
	}

	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(cmd.Environ(), "TERM=xterm-256color")
	// Let the harness coordinate a cooperative shutdown before forcing termination.
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second

	size := &pty.Winsize{
		Rows: 24,
		Cols: 80,
	}
	terminal, err := pty.StartWithSize(cmd, size)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start harness: %w", err)
	}

	p := &Process{
		cmd:    cmd,
		pty:    terminal,
		done:   make(chan struct{}),
		cancel: cancel,
	}
	go p.run(output)
	return p, nil
}

func (p *Process) run(output io.Writer) {
	defer close(p.done)
	defer p.cancel()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		// Linux returns EIO when the last PTY slave closes.
		_, _ = io.Copy(output, p.pty)
	}()

	p.err = p.cmd.Wait()

	// WaitDelay only kills the leader; also clean up any remaining group members.
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)

	// Drain final output, but do not wait indefinitely for an inherited PTY to close.
	select {
	case <-readDone:
	case <-time.After(250 * time.Millisecond):
		_ = p.pty.Close()
		<-readDone
	}
	_ = p.pty.Close()
}

// Wait returns the process exit result after the child and PTY reader have stopped.
func (p *Process) Wait() error {
	<-p.done
	return p.err
}

// Shutdown requests graceful termination and waits for cleanup.
// If ctx expires, it forces termination and returns ctx.Err(); call Wait to join cleanup.
// It is safe to call more than once.
func (p *Process) Shutdown(ctx context.Context) error {
	p.cancel()

	// Prefer completed cleanup if both done and ctx.Done are already closed.
	select {
	case <-p.done:
		return nil
	default:
	}

	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		return ctx.Err()
	}
}
