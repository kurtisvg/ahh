package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWrapStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	stdout, writer := io.Pipe()
	defer stdout.Close()
	var stderr bytes.Buffer

	cmd := NewCommand()
	cmd.SetArgs([]string{"wrap", "--address", "localhost", "--port", "0"})
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
	var started wrapStartup
	if err := json.NewDecoder(stdout).Decode(&started); err != nil {
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
	var output failedOutput
	cmd := newWrapCommand()
	cmd.SetContext(t.Context())
	cmd.SetOut(&output)

	opts := wrapOpts{
		address: "127.0.0.1",
		port:    0,
	}
	err := runWrapCommand(cmd, opts)
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
		{"wrap", "--address", "[invalid]", "--port", "0"},
		{"wrap", "--port", "-1"},
		{"wrap", "--port", "65536"},
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
