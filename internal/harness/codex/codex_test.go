package codex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	directory := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" \"$PWD\" \"$TERM\" > report\nexit 7\n"
	if err := os.WriteFile(filepath.Join(directory, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("TERM", "dumb")
	t.Chdir(directory)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	p, err := Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = p.Shutdown(context.Background())
		_ = p.Wait()
	}()

	// Codex startup preserves its exit status and uses the caller's directory.
	var exitError *exec.ExitError
	if err := p.Wait(); !errors.As(err, &exitError) || exitError.ExitCode() != 7 {
		t.Fatalf("exit = %v", err)
	}
	report, err := os.ReadFile("report")
	if err != nil {
		t.Fatal(err)
	}
	want := "--no-daemon\n" + directory + "\nxterm-256color\n"
	if string(report) != want {
		t.Fatalf("launch arguments, directory, and terminal = %q, want %q", report, want)
	}
}
