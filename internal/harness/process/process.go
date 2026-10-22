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
	"golang.org/x/sys/unix"
)

// Process owns one PTY and child process group. Create one with Start.
// The caller owns the output-reading goroutine and must call Shutdown to release
// the PTY, including after normal process exit.
//
// Use one reader and serialize Write and Resize calls. Reading may overlap input
// operations, and Shutdown may overlap all terminal I/O.
type Process struct {
	cmd *exec.Cmd
	pty *os.File

	done   chan struct{}
	cancel context.CancelFunc
	err    error // Read only after done closes.
}

// Start runs args[0] with args[1:] in a PTY. The caller must consume output.
// Cancel ctx to stop the child. Call Shutdown to release the PTY even after exit.
func Start(ctx context.Context, args []string) (*Process, error) {
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

	// PTY setup calls File.Fd, which switches the descriptor to blocking mode.
	// Restore nonblocking I/O so deadlines and Close can interrupt terminal writes.
	if err := syscall.SetNonblock(int(terminal.Fd()), true); err != nil {
		cancel()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = terminal.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("configure terminal I/O: %w", err)
	}

	p := &Process{
		cmd:    cmd,
		pty:    terminal,
		done:   make(chan struct{}),
		cancel: cancel,
	}
	go p.run(ctx)
	return p, nil
}

func (p *Process) run(ctx context.Context) {
	defer close(p.done)
	defer p.cancel()

	p.err = p.cmd.Wait()

	// WaitDelay only kills the leader; also clean up any remaining group members.
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)

	// Cancellation must unblock readers, even if a descendant retains the slave.
	// On normal exit, leave the PTY open so the caller can drain remaining output.
	if ctx.Err() != nil {
		_ = p.pty.Close()
	}
}

// Read reads terminal output. Use one reader; concurrent readers divide the stream.
// Wait does not consume or discard output. Shutdown unblocks a pending Read.
func (p *Process) Read(data []byte) (int, error) {
	n, err := p.pty.Read(data)
	// Linux returns EIO when the last slave closes; cleanup can also close the master.
	if errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed) {
		err = io.EOF
	}
	return n, err
}

// Write sends terminal input with a bounded write deadline.
// The caller must serialize Write and Resize calls.
func (p *Process) Write(data []byte) (int, error) {
	if err := p.pty.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, err
	}
	return p.pty.Write(data)
}

// Resize updates the terminal dimensions without changing its blocking mode.
// The caller must serialize Write and Resize calls.
func (p *Process) Resize(rows, cols uint16) error {
	raw, err := p.pty.SyscallConn()
	if err != nil {
		return err
	}

	size := &unix.Winsize{
		Row: rows,
		Col: cols,
	}
	var resizeErr error
	err = raw.Control(func(fd uintptr) {
		resizeErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, size)
	})
	if err != nil {
		return err
	}
	return resizeErr
}

// Wait returns the process exit result after the child is reaped and its group cleaned up.
// Drain remaining output and call Shutdown to close the PTY.
func (p *Process) Wait() error {
	<-p.done
	return p.err
}

// Shutdown requests graceful termination, waits for the child, and closes the PTY.
// Unread output may be discarded. Call this even after normal process exit.
// If ctx expires, it forces termination and returns ctx.Err(); call Wait to join cleanup.
// It is safe to call more than once.
func (p *Process) Shutdown(ctx context.Context) error {
	defer func() { _ = p.pty.Close() }()
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
