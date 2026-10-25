package harness

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestStartFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	// Failed dispatch must return a nil interface, not one containing a nil process.
	h, err := Start(t.Context(), "codex")
	if h != nil || !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("missing executable: harness = %v, error = %v", h, err)
	}

	h, err = Start(t.Context(), "unknown")
	if h != nil || err == nil || !strings.Contains(err.Error(), "unknown harness") {
		t.Fatalf("unknown name: harness = %v, error = %v", h, err)
	}
}
