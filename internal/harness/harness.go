// Package harness selects and starts supported coding harnesses.
package harness

import (
	"context"
	"fmt"

	"github.com/kurtisvg/ahh/internal/harness/codex"
)

// Harness is a running harness whose process and terminal share one lifetime.
type Harness interface {
	Wait() error
	Shutdown(context.Context) error
}

// Start launches the named harness in the current directory.
// Cancel ctx or call Shutdown to stop it, then call Wait to join cleanup.
func Start(ctx context.Context, name string) (Harness, error) {
	switch name {
	case "codex":
		h, err := codex.Start(ctx)
		if err != nil {
			return nil, fmt.Errorf("start %s: %w", name, err)
		}
		return h, nil
	default:
		return nil, fmt.Errorf("unknown harness %q", name)
	}
}
