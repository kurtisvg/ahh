// Package codex starts the Codex harness using its local installation and configuration.
package codex

import (
	"context"

	"github.com/kurtisvg/ahh/internal/harness/process"
)

// Start runs Codex in the current directory. Consume its output continuously
// and call Shutdown to release the terminal, even after process exit.
func Start(ctx context.Context) (*process.Process, error) {
	args := []string{"codex", "--no-daemon"}

	return process.Start(ctx, args)
}
