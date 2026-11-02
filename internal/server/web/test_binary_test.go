package web_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain builds before tests and closes afterward; only Get runs concurrently.
type testBinary struct {
	path      string
	directory string
}

// Build compiles Ahh unless a prebuilt executable is supplied.
func (b *testBinary) Build(prebuilt string) error {
	if b.path != "" {
		return errors.New("Ahh test binary is already prepared")
	}
	if prebuilt != "" {
		if !filepath.IsAbs(prebuilt) {
			return errors.New("prebuilt Ahh test binary requires an absolute path")
		}
		info, err := os.Stat(prebuilt)
		if err != nil {
			return fmt.Errorf("prebuilt Ahh test binary is unavailable at %q: %w", prebuilt, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("prebuilt Ahh test binary at %q is not an executable file", prebuilt)
		}
		b.path = prebuilt
		return nil
	}
	directory, err := os.MkdirTemp("", "ahh-browser-build-")
	if err != nil {
		return fmt.Errorf("create browser build directory: %w", err)
	}
	path := filepath.Join(directory, "ahh")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "build", "-o", path, ".")
	command.Dir = "../../.."
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.Join(
			fmt.Errorf("build Ahh test binary: %w\n%s", err, output),
			os.RemoveAll(directory),
		)
	}
	b.path, b.directory = path, directory
	return nil
}

func (b *testBinary) Get(t *testing.T) string {
	t.Helper()
	if b.path == "" {
		t.Fatal("Ahh test binary is not built; call Build before Get")
	}
	return b.path
}

func (b *testBinary) Close() error {
	if b.directory != "" {
		if err := os.RemoveAll(b.directory); err != nil {
			return fmt.Errorf("remove browser build directory: %w", err)
		}
	}
	b.path, b.directory = "", ""
	return nil
}

func TestBinaryUnbuilt(t *testing.T) {
	if os.Getenv("AHH_TEST_UNBUILT_BINARY") == "1" {
		var binary testBinary
		binary.Get(t)
		return
	}
	command := exec.CommandContext(t.Context(), os.Args[0],
		"-test.short", "-test.run=^TestBinaryUnbuilt$")
	command.Env = append(os.Environ(), "AHH_TEST_UNBUILT_BINARY=1")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Ahh test binary is not built") {
		t.Fatalf("expected unbuilt binary failure; got %v\n%s", err, output)
	}
}

func TestBinaryPreservesPrebuiltExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ahh")
	requireNoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	var binary testBinary
	requireNoError(t, binary.Build(path))
	got := binary.Get(t)
	if got != path {
		t.Fatalf("Get = %q, want %q", got, path)
	}
	requireNoError(t, binary.Close())
	_, err := os.Stat(path)
	requireNoError(t, err)
}

func TestBinaryRejectsInvalidPrebuilt(t *testing.T) {
	directory := t.TempDir()
	notExecutable := filepath.Join(directory, "ahh")
	requireNoError(t, os.WriteFile(notExecutable, []byte("not an executable"), 0o600))
	for _, tc := range []struct {
		name, path string
	}{
		{"relative", "ahh"},
		{"missing", filepath.Join(directory, "missing")},
		{"directory", directory},
		{"not executable", notExecutable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var binary testBinary
			if err := binary.Build(tc.path); err == nil {
				t.Fatal("Build accepted invalid prebuilt binary")
			}
		})
	}
}

func TestBinaryFailedBuildCleansUp(t *testing.T) {
	directory := t.TempDir()
	requireNoError(t, os.WriteFile(filepath.Join(directory, "go"),
		[]byte("#!/bin/sh\necho 'compiler failed' >&2\nexit 1\n"), 0o700))
	t.Setenv("PATH", directory)
	builds := t.TempDir()
	t.Setenv("TMPDIR", builds)
	var binary testBinary
	err := binary.Build("")
	if err == nil || !strings.Contains(err.Error(), "compiler failed") {
		t.Fatalf("Build = %v, want compiler output", err)
	}
	entries, err := os.ReadDir(builds)
	requireNoError(t, err)
	if len(entries) != 0 {
		t.Fatalf("failed build left %d files", len(entries))
	}
}
