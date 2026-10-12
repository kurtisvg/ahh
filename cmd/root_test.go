package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/version"
)

// The version flag should report the embedded version on stdout without diagnostics.
func TestVersion(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := executeRootCommand(t.Context(), "--version")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stdout, "ahh version "+version.Version+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q", stderr)
	}
}

// The root and help flags should show usage without advertising completions.
func TestHelp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{name: "root"},
		{name: "help flag", args: []string{"--help"}},
		{name: "short help flag", args: []string{"-h"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := executeRootCommand(t.Context(), tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout, "Usage:") || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
			}
			if strings.Contains(stdout, "completion") {
				t.Error("help advertises automatic completions")
			}
		})
	}
}

// Commands should return errors without printing them so Execute reports them once.
func TestInvalidInput(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"missing"}, {"--missing"}, {"version"}, {"completion"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := executeRootCommand(t.Context(), args...)
			if err == nil {
				t.Fatal("expected an error")
			}
			if stdout != "" || stderr != "" {
				t.Fatalf("command printed before Execute handled the error: %q %q", stdout, stderr)
			}
		})
	}
}

func executeRootCommand(ctx context.Context, args ...string) (string, string, error) {
	cmd := NewCommand()
	var stdout, stderr bytes.Buffer
	// An empty slice prevents Cobra from reading the test binary's arguments.
	cmd.SetArgs(append([]string{}, args...))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.ExecuteContext(ctx)
	return stdout.String(), stderr.String(), err
}
