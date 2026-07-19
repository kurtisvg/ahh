package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/creack/pty"
)

const (
	defaultRows uint16 = 24
	defaultCols uint16 = 80
)

var claudeCommand = "claude"

type options struct {
	configDir string
}

// Option configures a Claude Code harness process.
type Option func(*options)

// WithConfigDir isolates Claude Code configuration and history in dir.
func WithConfigDir(dir string) Option {
	return func(opts *options) {
		opts.configDir = strings.TrimSpace(dir)
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
func Start(ctx context.Context, sessionID string, startOpts ...Option) (Harness, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	opts := options{}
	for _, opt := range startOpts {
		opt(&opts)
	}
	args, err := claudeArguments(sessionID, opts)
	if err != nil {
		return nil, fmt.Errorf("resolve claude-code session: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, claudeCommand, args...)
	cmd.Env = claudeEnvironment(os.Environ(), opts.configDir)

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

func claudeArguments(sessionID string, opts options) ([]string, error) {
	transcriptExists, err := claudeTranscriptExists(opts.configDir, sessionID)
	if err != nil {
		return nil, err
	}
	if transcriptExists {
		return []string{"--resume", sessionID}, nil
	}

	return []string{"--session-id", sessionID}, nil
}

func claudeTranscriptExists(configDir, sessionID string) (bool, error) {
	if configDir == "" {
		return false, nil
	}
	projectsDir := filepath.Join(configDir, "projects")
	projects, err := os.ReadDir(projectsDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read claude projects directory %q: %w", projectsDir, err)
	}
	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		transcriptPath := filepath.Join(projectsDir, project.Name(), sessionID+".jsonl")
		info, err := os.Stat(transcriptPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("stat claude transcript %q: %w", transcriptPath, err)
		}
		if info.IsDir() {
			return false, fmt.Errorf("claude transcript %q is not a file", transcriptPath)
		}

		return true, nil
	}

	return false, nil
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

func claudeEnvironment(env []string, configDir string) []string {
	skip := map[string]struct{}{
		"CLICOLOR":    {},
		"COLORTERM":   {},
		"FORCE_COLOR": {},
		"NO_COLOR":    {},
		"TERM":        {},
	}
	if configDir != "" {
		skip["CLAUDE_CONFIG_DIR"] = struct{}{}
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

	next = append(
		next,
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"CLICOLOR=1",
		"FORCE_COLOR=1",
	)
	if configDir != "" {
		next = append(next, "CLAUDE_CONFIG_DIR="+configDir)
	}
	return next
}
