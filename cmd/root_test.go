package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/version"

	"github.com/spf13/cobra"
)

func invokeCommand(t *testing.T, args []string) (*cobra.Command, string, error) {
	t.Helper()

	buf := new(bytes.Buffer)
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	executed, err := cmd.ExecuteC()
	return executed, buf.String(), err
}

func invokeCommandWithoutRun(t *testing.T, args []string) (*cobra.Command, string, error) {
	t.Helper()

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

func flagString(t *testing.T, cmd *cobra.Command, name string) string {
	t.Helper()

	value, err := cmd.Flags().GetString(name)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRootCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		args           []string
		wantOutput     string
		wantOutputPart string
		wantErr        bool
	}{
		{
			name:       "version",
			args:       []string{"--version"},
			wantOutput: "ahh version " + version.Version + "\n",
		},
		{
			name:           "missing command",
			wantOutputPart: "Usage:",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, output, err := invokeCommand(t, tt.args)
			if tt.wantErr && err == nil {
				t.Fatal("error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatal(err)
			}
			if tt.wantOutput != "" && output != tt.wantOutput {
				t.Fatalf("output = %q, want %q", output, tt.wantOutput)
			}
			if tt.wantOutputPart != "" && !strings.Contains(output, tt.wantOutputPart) {
				t.Fatalf("output = %q, want substring %q", output, tt.wantOutputPart)
			}
		})
	}
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

			cmd, _, err := invokeCommandWithoutRun(t, tt.args)
			if err != nil {
				t.Fatal(err)
			}

			if got := cmd.CommandPath(); got != tt.wantPath {
				t.Fatalf("command path = %q, want %q", got, tt.wantPath)
			}

			if got := flagString(t, cmd, "host"); got != tt.wantHost {
				t.Fatalf("host = %q, want %q", got, tt.wantHost)
			}
			if got := flagString(t, cmd, "port"); got != tt.wantPort {
				t.Fatalf("port = %q, want %q", got, tt.wantPort)
			}
		})
	}
}

func TestRunCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantPath string
	}{
		{
			name:     "claude code",
			args:     []string{"run", "claude-code"},
			wantPath: "ahh run claude-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd, _, err := invokeCommandWithoutRun(t, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := cmd.CommandPath(); got != tt.wantPath {
				t.Fatalf("command path = %q, want %q", got, tt.wantPath)
			}
		})
	}
}
