package wrapper

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/kurtisvg/ahh/internal/harness"
)

// copyHarnessOutput owns the single output reader and releases the harness terminal
// after a bounded final drain. It returns copy errors; process errors remain available
// through h.Wait. The destination stays owned by the caller and is never closed here.
// Writes must return promptly; browser connections consume separate queues.
func copyHarnessOutput(ctx context.Context, h harness.Harness, output io.Writer) error {
	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = h.Shutdown(ctx)
	}

	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(output, h)
		if err != nil {
			// A failed consumer must not leave the child running without a reader.
			shutdown()
			err = fmt.Errorf("copy harness output: %w", err)
		}
		done <- err
	}()

	_ = h.Wait()
	defer shutdown()

	// Drain final output before closing the PTY. An inherited slave must not
	// keep the reader alive indefinitely after the harness exits.
	select {
	case err := <-done:
		return err
	case <-time.After(250 * time.Millisecond):
		shutdown()
		return <-done
	}
}
