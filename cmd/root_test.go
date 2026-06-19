package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestParseCommandServer(t *testing.T) {
	t.Parallel()

	cmd, err := parseCommand([]string{"server", "--host", "localhost", "--port", "18080"})
	if err != nil {
		t.Fatal(err)
	}

	if cmd.name != "server" {
		t.Fatalf("name = %q, want %q", cmd.name, "server")
	}
	if cmd.serverOptions.host != "localhost" {
		t.Fatalf("host = %q, want %q", cmd.serverOptions.host, "localhost")
	}
	if cmd.serverOptions.port != "18080" {
		t.Fatalf("port = %q, want %q", cmd.serverOptions.port, "18080")
	}
}

func TestParseCommandVersion(t *testing.T) {
	t.Parallel()

	cmd, err := parseCommand([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}

	if cmd.name != "version" {
		t.Fatalf("name = %q, want %q", cmd.name, "version")
	}
}

func TestParseCommandRunWrapper(t *testing.T) {
	t.Parallel()

	cmd, err := parseCommand([]string{"run-wrapper", "claude-code"})
	if err != nil {
		t.Fatal(err)
	}

	if cmd.name != "run-wrapper" {
		t.Fatalf("name = %q, want %q", cmd.name, "run-wrapper")
	}
}

func TestRunWithoutCommandShowsUsage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(context.Background(), nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("run returned nil error, want missing command error")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "Usage:") {
		t.Fatalf("stderr = %q, want usage", got)
	}
}
