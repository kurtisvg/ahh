// Package codex starts the Codex harness using its local installation and configuration.
package codex

import (
	"context"
	"io"

	"github.com/kurtisvg/ahh/internal/harness/process"
)

// Start runs Codex in the current directory. Cancel ctx or call Shutdown to stop it.
func Start(ctx context.Context) (*process.Process, error) {
	args := []string{"codex", "--no-daemon"}

	return process.Start(ctx, io.Discard, args)
}
