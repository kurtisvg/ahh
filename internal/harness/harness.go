package harness

import (
	"context"
	"io"
)

// Type identifies a harness implementation.
type Type string

// ClaudeCode identifies the Claude Code harness.
const ClaudeCode Type = "claude-code"

// Harness owns the lifecycle of an underlying coding-agent harness process.
type Harness interface {
	io.Reader
	io.Writer
	Wait(context.Context) error
	Done() bool
	Resize(rows, cols uint16) error
	Close()
}
