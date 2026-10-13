package cmd

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServeStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stdout, writer := io.Pipe()
	defer stdout.Close()

	cmd := NewCommand()
	cmd.SetArgs([]string{"serve", "--address", "localhost", "--port", "0"})
	cmd.SetOut(writer)
	cmd.SetErr(io.Discard)

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

	// The announced URL must contain the assigned port and already serve requests.
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	url := strings.TrimSpace(line)
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

	cancel()
	<-done
	if runErr != nil {
		t.Fatal(runErr)
	}
}

func TestServeStartupOutputFailureStopsServer(t *testing.T) {
	var output failedOutput
	cmd := newServeCommand()
	cmd.SetContext(t.Context())
	cmd.SetOut(&output)

	opts := serveOpts{
		address: "127.0.0.1",
		port:    0,
	}
	if err := runServeCommand(cmd, opts); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("startup output failure = %v", err)
	}

	address := strings.TrimPrefix(strings.TrimSpace(output.String()), "http://")
	if conn, err := net.DialTimeout("tcp", address, time.Second); err == nil {
		conn.Close()
		t.Fatal("failed startup announcement left server running")
	}
}

func TestServeInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"serve", "extra"},
		{"serve", "--address", "[invalid]", "--port", "0"},
		{"serve", "--port", "-1"},
		{"serve", "--port", "65536"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := executeRootCommand(t.Context(), args...)
			if err == nil {
				t.Fatal("expected invalid input error")
			}
			if stdout != "" || stderr != "" {
				t.Fatalf("announced startup for invalid input: %q %q", stdout, stderr)
			}
		})
	}
}
