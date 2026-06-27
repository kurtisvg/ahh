package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

const (
	defaultRows uint16 = 24
	defaultCols uint16 = 80
)

var claudeCommand = "claude"

// ClaudeCodeHarness owns a Claude Code PTY subprocess.
type ClaudeCodeHarness struct {
	cancel context.CancelFunc
	pty    *os.File
	// done is closed when the harness exits. runErr is set before done closes,
	// so callers must observe done before reading runErr.
	done   chan struct{}
	runErr error
}

// Start starts a Claude Code subprocess inside a PTY.
func Start(ctx context.Context) (Harness, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, claudeCommand)

	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: defaultRows,
		Cols: defaultCols,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start claude-code pty: %w", err)
	}

	h := &ClaudeCodeHarness{
		cancel: cancel,
		pty:    ptyFile,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(h.done)
		defer cancel()

		err := cmd.Wait()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = ctxErr
			} else {
				err = fmt.Errorf("pty command %q exited: %w", cmd.Path, err)
			}
		}

		h.runErr = err
	}()

	return h, nil
}

func (h *ClaudeCodeHarness) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.done:
	}

	return h.runErr
}

func (h *ClaudeCodeHarness) Done() bool {
	select {
	case <-h.done:
		return true
	default:
		return false
	}
}

func (h *ClaudeCodeHarness) Close() {
	h.cancel()
	_ = h.pty.Close()
}
