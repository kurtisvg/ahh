package harness

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

var claudeAgentACPCommand = "claude-agent-acp"

// ClaudeCodeHarness owns a Claude Code ACP subprocess.
type ClaudeCodeHarness struct {
	cancel  context.CancelFunc
	closers []io.Closer
	// done is closed when the harness exits. runErr is set before done closes,
	// so callers must observe done before reading runErr.
	done   chan struct{}
	runErr error
}

// StartClaudeCode starts a Claude Code ACP subprocess.
func StartClaudeCode(ctx context.Context) (Harness, error) {
	ctx, cancel := context.WithCancel(ctx)
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open harness stdin pipe: %w", err)
	}

	cmd := exec.CommandContext(ctx, claudeAgentACPCommand)
	cmd.Stdin = pr

	h := &ClaudeCodeHarness{
		cancel:  cancel,
		closers: []io.Closer{pr, pw},
		done:    make(chan struct{}),
	}

	go func() {
		defer close(h.done)
		defer cancel()

		err := cmd.Run()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = ctxErr
			} else {
				err = fmt.Errorf("harness %q exited: %w", cmd.Path, err)
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
	for _, closer := range h.closers {
		_ = closer.Close()
	}
}
