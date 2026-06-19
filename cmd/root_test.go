package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/version"

	"github.com/spf13/cobra"
)

func invokeCommand(args []string) (*cobra.Command, string, error) {
	buf := new(bytes.Buffer)
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	executed, err := cmd.ExecuteC()
	return executed, buf.String(), err
}

func invokeCommandWithoutRun(args []string) (*cobra.Command, string, error) {
	buf := new(bytes.Buffer)
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if target, _, err := cmd.Find(args); err == nil {
		target.Run = nil
		target.RunE = func(*cobra.Command, []string) error {
			return nil
		}
	}
	cmd.SetArgs(args)
	executed, err := cmd.ExecuteC()
	return executed, buf.String(), err
}

func TestServerCommandPassesOptions(t *testing.T) {
	t.Parallel()

	cmd, _, err := invokeCommandWithoutRun([]string{"serve", "--host", "localhost", "--port", "18080"})
	if err != nil {
		t.Fatal(err)
	}

	host, err := cmd.Flags().GetString("host")
	if err != nil {
		t.Fatal(err)
	}
	if host != "localhost" {
		t.Fatalf("host = %q, want %q", host, "localhost")
	}
	port, err := cmd.Flags().GetString("port")
	if err != nil {
		t.Fatal(err)
	}
	if port != "18080" {
		t.Fatalf("port = %q, want %q", port, "18080")
	}
}

func TestRunVersion(t *testing.T) {
	t.Parallel()

	_, got, err := invokeCommand([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "ahh version " + version.Version + "\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunWrapperCommandPassesOptions(t *testing.T) {
	t.Parallel()

	cmd, _, err := invokeCommandWithoutRun([]string{"run-wrapper", "claude-code"})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := cmd.CommandPath(), "ahh run-wrapper claude-code"; got != want {
		t.Fatalf("command path = %q, want %q", got, want)
	}
}

func TestRunWithoutCommandShowsUsage(t *testing.T) {
	t.Parallel()

	_, output, err := invokeCommand(nil)
	if err == nil {
		t.Fatal("run returned nil error, want missing command error")
	}
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("output = %q, want usage", output)
	}
}
