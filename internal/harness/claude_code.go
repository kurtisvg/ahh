package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
)

const (
	defaultRows uint16 = 24
	defaultCols uint16 = 80
)

var claudeCommand = "claude"

type options struct {
	sessionID   string
	sessionName string
	resume      bool
	configured  bool
}

// Option configures a Claude Code harness process.
type Option func(*options) error

// WithNewSession configures Claude Code to create a session with id and name.
func WithNewSession(id, name string) Option {
	return func(opts *options) error {
		if id == "" {
			return fmt.Errorf("session id is required")
		}
		if opts.configured {
			return fmt.Errorf("session start mode is already configured")
		}
		opts.sessionID = id
		opts.sessionName = name
		opts.configured = true
		return nil
	}
}

// WithResumeSession configures Claude Code to resume the session with id.
func WithResumeSession(id string) Option {
	return func(opts *options) error {
		if id == "" {
			return fmt.Errorf("session id is required")
		}
		if opts.configured {
			return fmt.Errorf("session start mode is already configured")
		}
		opts.sessionID = id
		opts.resume = true
		opts.configured = true
		return nil
	}
}

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
func Start(ctx context.Context, startOpts ...Option) (Harness, error) {
	opts := options{}
	for _, opt := range startOpts {
		if err := opt(&opts); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, claudeCommand, claudeArguments(opts)...)
	cmd.Env = claudeEnvironment(os.Environ())

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

func claudeArguments(opts options) []string {
	if opts.resume {
		return []string{"--resume", opts.sessionID}
	}
	if opts.sessionID == "" {
		return nil
	}

	args := []string{"--session-id", opts.sessionID}
	if opts.sessionName != "" {
		args = append(args, "--name", opts.sessionName)
	}

	return args
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

func (h *ClaudeCodeHarness) Read(p []byte) (int, error) {
	return h.pty.Read(p)
}

func (h *ClaudeCodeHarness) Write(p []byte) (int, error) {
	return h.pty.Write(p)
}

func (h *ClaudeCodeHarness) Resize(rows, cols uint16) error {
	if rows == 0 || cols == 0 {
		return fmt.Errorf("resize pty: rows and cols must be positive")
	}

	return pty.Setsize(h.pty, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

func (h *ClaudeCodeHarness) Close() {
	h.cancel()
	_ = h.pty.Close()
}

func claudeEnvironment(env []string) []string {
	skip := map[string]struct{}{
		"CLICOLOR":    {},
		"COLORTERM":   {},
		"FORCE_COLOR": {},
		"NO_COLOR":    {},
		"TERM":        {},
	}

	next := make([]string, 0, len(env)+4)
	for _, value := range env {
		key, _, ok := strings.Cut(value, "=")
		if !ok {
			next = append(next, value)
			continue
		}
		if _, ok := skip[key]; ok {
			continue
		}
		next = append(next, value)
	}

	return append(
		next,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"CLICOLOR=1",
		"FORCE_COLOR=1",
	)
}
