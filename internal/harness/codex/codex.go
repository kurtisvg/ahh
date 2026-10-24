// Package codex starts the Codex harness using its local installation and configuration.
package codex

import (
	"context"

	"github.com/kurtisvg/ahh/internal/harness/pty"
)

// Harness runs Codex in a PTY and provides terminal I/O and process cleanup.
type Harness struct {
	*pty.Process
}

// Start runs Codex in the current directory. Consume its output continuously
// and call Shutdown to release the terminal, even after process exit.
func Start(ctx context.Context) (*Harness, error) {
	args := []string{"codex", "--no-daemon"}

	p, err := pty.Start(ctx, args)
	if err != nil {
		return nil, err
	}

	return &Harness{Process: p}, nil
}
