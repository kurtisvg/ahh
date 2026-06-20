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

func TestRootCommand(t *testing.T) {
	t.Parallel()

	t.Run("version", func(t *testing.T) {
		t.Parallel()

		_, got, err := invokeCommand([]string{"--version"})
		if err != nil {
			t.Fatal(err)
		}
		if want := "ahh version " + version.Version + "\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("missing command", func(t *testing.T) {
		t.Parallel()

		_, output, err := invokeCommand(nil)
		if err == nil {
			t.Fatal("run returned nil error, want missing command error")
		}
		if !strings.Contains(output, "Usage:") {
			t.Fatalf("output = %q, want usage", output)
		}
	})
}

func TestServeCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantPath string
		wantHost string
		wantPort string
	}{
		{
			name:     "defaults",
			args:     []string{"serve"},
			wantPath: "ahh serve",
			wantHost: "127.0.0.1",
			wantPort: "8080",
		},
		{
			name:     "configured address",
			args:     []string{"serve", "--host", "localhost", "--port", "18080"},
			wantPath: "ahh serve",
			wantHost: "localhost",
			wantPort: "18080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd, _, err := invokeCommandWithoutRun(tt.args)
			if err != nil {
				t.Fatal(err)
			}

			if got := cmd.CommandPath(); got != tt.wantPath {
				t.Fatalf("command path = %q, want %q", got, tt.wantPath)
			}

			host, err := cmd.Flags().GetString("host")
			if err != nil {
				t.Fatal(err)
			}
			if host != tt.wantHost {
				t.Fatalf("host = %q, want %q", host, tt.wantHost)
			}

			port, err := cmd.Flags().GetString("port")
			if err != nil {
				t.Fatal(err)
			}
			if port != tt.wantPort {
				t.Fatalf("port = %q, want %q", port, tt.wantPort)
			}
		})
	}
}

func TestRunCommand(t *testing.T) {
	t.Parallel()

	cmd, _, err := invokeCommandWithoutRun([]string{"run", "claude-code"})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := cmd.CommandPath(), "ahh run claude-code"; got != want {
		t.Fatalf("command path = %q, want %q", got, want)
	}
}
