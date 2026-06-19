package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/version"

	"github.com/spf13/cobra"
)

func invokeCommand(args []string, runners commandRunners) (*cobra.Command, string, error) {
	buf := new(bytes.Buffer)
	cmd := newRootCommand(context.Background(), buf, buf, runners)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return cmd, buf.String(), err
}

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

	_, _, err := invokeCommand([]string{"serve", "--host", "localhost", "--port", "18080"}, runners)
	if err != nil {
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

	_, got, err := invokeCommand([]string{"--version"}, commandRunners{
		server:  runServer,
		wrapper: runWrapper,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "ahh version " + version.Version + "\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
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

	_, _, err := invokeCommand([]string{"run-wrapper", "claude-code"}, runners)
	if err != nil {
		t.Fatal(err)
	}

	if got.harness != "claude-code" {
		t.Fatalf("harness = %q, want %q", got.harness, "claude-code")
	}
}

func TestRunWithoutCommandShowsUsage(t *testing.T) {
	t.Parallel()

	_, output, err := invokeCommand(nil, commandRunners{
		server:  runServer,
		wrapper: runWrapper,
	})
	if err == nil {
		t.Fatal("run returned nil error, want missing command error")
	}
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("output = %q, want usage", output)
	}
}
