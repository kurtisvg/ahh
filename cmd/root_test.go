package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/version"
)

func TestServerCommandPassesOptions(t *testing.T) {
	t.Parallel()

	var got serverOptions
	runners := commandRunners{
		server: func(_ context.Context, _ io.Writer, opts serverOptions) error {
			got = opts
			return nil
		},
		wrapper: runWrapper,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runWithRunners(context.Background(), []string{"server", "--host", "localhost", "--port", "18080"}, &stdout, &stderr, runners); err != nil {
		t.Fatal(err)
	}

	if got.host != "localhost" {
		t.Fatalf("host = %q, want %q", got.host, "localhost")
	}
	if got.port != "18080" {
		t.Fatalf("port = %q, want %q", got.port, "18080")
	}
}

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run(context.Background(), []string{"--version"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), version.Version+"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunWrapperCommandPassesOptions(t *testing.T) {
	t.Parallel()

	var got wrapperOptions
	runners := commandRunners{
		server: runServer,
		wrapper: func(_ context.Context, _ io.Writer, opts wrapperOptions) error {
			got = opts
			return nil
		},
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runWithRunners(context.Background(), []string{"run-wrapper", "claude-code"}, &stdout, &stderr, runners); err != nil {
		t.Fatal(err)
	}

	if got.harness != "claude-code" {
		t.Fatalf("harness = %q, want %q", got.harness, "claude-code")
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
