// Package harness selects and starts supported coding harnesses.
//
// A harness owns its child process and terminal. Wait reports process exit;
// remaining output stays readable until Shutdown releases the terminal.
package harness

import (
	"context"
	"fmt"
	"io"

	"github.com/kurtisvg/ahh/internal/harness/codex"
)

// Harness supports a single reader and a single writer, which may run concurrently.
// Concurrent writes or resize operations are not safe; callers must serialize them.
// Shutdown may run concurrently with terminal I/O.
type Harness interface {
	io.ReadWriter
	Resize(rows, cols uint16) error

	// Wait reports process exit; output may still remain to be read.
	Wait() error

	// Shutdown stops the process and closes the terminal, unblocking I/O.
	Shutdown(context.Context) error
}

// Start launches the named harness in the current directory.
// Consume output continuously and call Shutdown to release the terminal, even after exit.
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
