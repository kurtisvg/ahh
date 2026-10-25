package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWrapStartup(t *testing.T) {
	useFakeCodex(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	stdout, writer := io.Pipe()
	defer stdout.Close()
	var stderr bytes.Buffer

	cmd := NewCommand()
	cmd.SetArgs([]string{"wrap", "codex", "--address", "localhost", "--port", "0"})
	cmd.SetOut(writer)
	cmd.SetErr(&stderr)

	// Run the command in the background so we can connect while it is serving.
	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		defer writer.Close()
		runErr = cmd.ExecuteContext(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		_ = stdout.Close()
		<-done
	})

	// Startup JSON must report the resolved address and assigned port.
	decoder := json.NewDecoder(stdout)
	var started wrapStartup
	if err := decoder.Decode(&started); err != nil {
		t.Fatal(err)
	}
	if !net.ParseIP(started.Address).IsLoopback() || started.Port == 0 {
		t.Fatalf("startup = %+v", started)
	}

	// The announced endpoint should already be ready to handle requests.
	url := "http://" + net.JoinHostPort(started.Address, strconv.Itoa(started.Port))
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(body) != "hello world\n" {
		t.Fatalf("response = %q, error = %v", body, err)
	}

	// Wait for shutdown before checking the result and reading captured stderr.
	cancel()
	var finished struct {
		Type string `json:"type"`
	}
	if err := decoder.Decode(&finished); err != nil || finished.Type != "exit" {
		t.Fatalf("exit report = %+v, error = %v", finished, err)
	}
	// Consume the final newline so the pipe writer can finish its write.
	if _, err := io.Copy(io.Discard, stdout); err != nil {
		t.Fatal(err)
	}
	<-done
	if runErr != nil || !strings.Contains(stderr.String(), url) {
		t.Fatalf("command error = %v, diagnostics = %q", runErr, stderr.String())
	}
}

type failedOutput struct {
	bytes.Buffer
}

func (w *failedOutput) Write(p []byte) (int, error) {
	_, _ = w.Buffer.Write(p)
	return 0, io.ErrClosedPipe
}

func TestWrapStartupOutputFailureStopsServer(t *testing.T) {
	useFakeCodex(t)

	var output failedOutput
	cmd := newWrapCommand()
	cmd.SetContext(t.Context())
	cmd.SetOut(&output)

	opts := wrapOpts{
		address: "127.0.0.1",
		port:    0,
	}
	err := runWrapCommand(cmd, opts, "codex")
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("startup output failure = %v", err)
	}

	var started wrapStartup
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}

	address := net.JoinHostPort(started.Address, strconv.Itoa(started.Port))
	if conn, err := net.DialTimeout("tcp", address, time.Second); err == nil {
		conn.Close()
		t.Fatal("failed startup announcement left server running")
	}
}

func TestWrapInvalidAddress(t *testing.T) {
	for _, args := range [][]string{
		{"wrap", "codex", "--address", "[invalid]", "--port", "0"},
		{"wrap", "codex", "--port", "-1"},
		{"wrap", "codex", "--port", "65536"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := executeRootCommand(t.Context(), args...)
			if err == nil || !strings.Contains(err.Error(), "start wrapper server") {
				t.Fatalf("invalid bind address error = %v", err)
			}

			if stdout != "" || stderr != "" {
				t.Fatalf("announced startup for invalid address: %q %q", stdout, stderr)
			}
		})
	}
}

func TestWrapLaunchFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := executeRootCommand(t.Context(), "wrap", "codex", "--port", "0")
	if err == nil || !strings.Contains(err.Error(), "start harness") {
		t.Fatalf("launch failure = %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("announced startup after launch failure: %q %q", stdout, stderr)
	}
}

func TestWrapInvalidHarness(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	for _, args := range [][]string{
		{"wrap"},
		{"wrap", "unknown"},
		{"wrap", "codex", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := executeRootCommand(t.Context(), args...)
			if err == nil || strings.Contains(err.Error(), "start wrapper server") {
				t.Fatalf("expected argument validation error, got %v", err)
			}
			if stdout != "" || stderr != "" {
				t.Fatalf("announced startup for invalid harness: %q %q", stdout, stderr)
			}
		})
	}
}

func useFakeCodex(t *testing.T) {
	t.Helper()

	script, err := os.ReadFile("../internal/harness/pty/testdata/fake-harness")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "codex"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}
