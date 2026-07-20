package harness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const fakeHarnessScript = `#!/bin/sh
case "${AHH_HELPER_HARNESS_MODE}" in
  success)
    exit 0
    ;;
  failure)
    exit 7
    ;;
  wait)
    sleep 10
    ;;
  inspect)
    printf '%s\n%s\n' "${PWD}" "${AHH_INTERNAL_GIT_TEST}"
    ;;
  *)
    printf 'unknown helper harness mode "%s"\n' "${AHH_HELPER_HARNESS_MODE}" >&2
    exit 2
    ;;
esac
`

func TestHarness(t *testing.T) {
	tests := []struct {
		name                 string
		startHarness         func(*testing.T, context.Context) (Harness, error)
		wantStartErrContains string
		wantWaitErrContains  string
	}{
		{
			name: "runs harness to completion",
			startHarness: func(t *testing.T, ctx context.Context) (Harness, error) {
				return fakeHarness(t, ctx, "success")
			},
		},
		{
			name: "reports missing command",
			startHarness: func(t *testing.T, ctx context.Context) (Harness, error) {
				setHarnessCommand(t, filepath.Join(t.TempDir(), "missing-claude"))
				return Start(ctx, "test-session-id")
			},
			wantStartErrContains: "no such file or directory",
		},
		{
			name: "reports failing harness",
			startHarness: func(t *testing.T, ctx context.Context) (Harness, error) {
				return fakeHarness(t, ctx, "failure")
			},
			wantWaitErrContains: `pty command "`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness, err := tt.startHarness(t, t.Context())
			if tt.wantStartErrContains != "" {
				if err == nil {
					t.Fatal("Start() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantStartErrContains) {
					t.Fatalf("Start() error = %q, want containing %q", err.Error(), tt.wantStartErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			t.Cleanup(harness.Close)
			err = harness.Wait(t.Context())

			if tt.wantWaitErrContains == "" && err != nil {
				t.Fatalf("Wait() error = %v", err)
			}
			if tt.wantWaitErrContains != "" {
				if err == nil {
					t.Fatal("Wait() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantWaitErrContains) {
					t.Fatalf("Wait() error = %q, want containing %q", err.Error(), tt.wantWaitErrContains)
				}
			}
		})
	}
}

func TestHarnessWaitReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	harness, err := fakeHarness(t, ctx, "wait")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(harness.Close)
	err = harness.Wait(t.Context())

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestHarnessDone(t *testing.T) {
	harness, err := fakeHarness(t, t.Context(), "wait")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if harness.Done() {
		t.Fatal("Done() = true before Wait(), want false")
	}
	harness.Close()
	if err := harness.Wait(t.Context()); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v", err)
	}
	if !harness.Done() {
		t.Fatal("Done() = false after Wait(), want true")
	}
}

func TestClaudeEnvironmentEnablesColor(t *testing.T) {
	env := claudeEnvironment([]string{
		"PATH=/bin",
		"NO_COLOR=1",
		"TERM=dumb",
		"COLORTERM=",
		"CLICOLOR=0",
		"FORCE_COLOR=0",
		"VSCODE_IPC_HOOK_CLI=/tmp/vscode.sock",
	}, "")
	got := envMap(env)

	if got["NO_COLOR"] != "" {
		t.Fatalf("NO_COLOR = %q, want unset", got["NO_COLOR"])
	}
	if got["TERM"] != "xterm-256color" {
		t.Fatalf("TERM = %q, want xterm-256color", got["TERM"])
	}
	if got["COLORTERM"] != "truecolor" {
		t.Fatalf("COLORTERM = %q, want truecolor", got["COLORTERM"])
	}
	if got["CLICOLOR"] != "1" {
		t.Fatalf("CLICOLOR = %q, want 1", got["CLICOLOR"])
	}
	if got["FORCE_COLOR"] != "1" {
		t.Fatalf("FORCE_COLOR = %q, want 1", got["FORCE_COLOR"])
	}
	if got["VSCODE_IPC_HOOK_CLI"] != "/tmp/vscode.sock" {
		t.Fatalf("VSCODE_IPC_HOOK_CLI = %q, want preserved", got["VSCODE_IPC_HOOK_CLI"])
	}
}

func TestClaudeEnvironmentUsesManagedConfigDirectory(t *testing.T) {
	env := claudeEnvironment([]string{
		"PATH=/bin",
		"CLAUDE_CONFIG_DIR=/inherited",
	}, "/managed/agent/config")
	got := envMap(env)

	if got["CLAUDE_CONFIG_DIR"] != "/managed/agent/config" {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want managed Agent directory", got["CLAUDE_CONFIG_DIR"])
	}
}

func TestWithConfigDirTrimsSpace(t *testing.T) {
	opts := options{}
	WithConfigDir("  /managed/agent/config  ")(&opts)

	if opts.configDir != "/managed/agent/config" {
		t.Fatalf("configDir = %q, want trimmed managed Agent directory", opts.configDir)
	}
}

func TestHarnessUsesWorkingDirectoryAndInternalEnvironment(t *testing.T) {
	workingDir := t.TempDir()
	setHarnessCommand(t, fakeHarnessCommand(t))
	t.Setenv("AHH_HELPER_HARNESS_MODE", "inspect")
	t.Setenv("AHH_INTERNAL_GIT_TEST", "ambient")

	h, err := Start(
		t.Context(),
		"test-session-id",
		WithWorkingDirectory("  "+workingDir+"  "),
		WithEnvironment([]string{"AHH_INTERNAL_GIT_TEST=managed"}),
	)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(h.Close)
	buffer := make([]byte, 4096)
	n, readErr := h.Read(buffer)
	if readErr != nil {
		t.Fatalf("Read() error = %v", readErr)
	}
	output := string(buffer[:n])
	if !strings.Contains(output, workingDir) {
		t.Fatalf("harness output = %q, want working directory %q", output, workingDir)
	}
	if !strings.Contains(output, "managed") || strings.Contains(output, "ambient") {
		t.Fatalf("harness output = %q, want managed environment override", output)
	}
	if err := h.Wait(t.Context()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestMergeEnvironmentOverridesOnlyMatchingKeys(t *testing.T) {
	actual := mergeEnvironment(
		[]string{"PATH=/bin", "GIT_SSH_COMMAND=ambient", "KEEP=value"},
		[]string{"GIT_SSH_COMMAND=managed", "GIT_TERMINAL_PROMPT=0"},
	)
	got := envMap(actual)
	if got["GIT_SSH_COMMAND"] != "managed" || got["GIT_TERMINAL_PROMPT"] != "0" || got["KEEP"] != "value" {
		t.Fatalf("mergeEnvironment() = %q", actual)
	}
}

// fakeHarness points Start at a temporary executable so tests exercise
// the real PTY subprocess lifecycle without requiring Claude Code to be installed.
func fakeHarness(t *testing.T, ctx context.Context, mode string) (Harness, error) {
	t.Helper()

	setHarnessCommand(t, fakeHarnessCommand(t))
	t.Setenv("AHH_HELPER_HARNESS_MODE", mode)

	return Start(ctx, "test-session-id")
}

func TestClaudeArguments(t *testing.T) {
	const sessionID = "fbce6273-3e75-4288-a89a-38f36f0cc0d1"
	writeTranscript := func(data []byte) func(*testing.T, string) {
		return func(t *testing.T, configDir string) {
			t.Helper()

			projectDir := filepath.Join(configDir, "projects", "test-project")
			if err := os.MkdirAll(projectDir, 0o700); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			transcriptPath := filepath.Join(projectDir, sessionID+".jsonl")
			if err := os.WriteFile(transcriptPath, data, 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
		}
	}
	tests := []struct {
		name            string
		useConfigDir    bool
		configure       func(*testing.T, string)
		want            []string
		wantErrContains string
	}{
		{
			name: "starts conversation",
			want: []string{"--session-id", sessionID},
		},
		{
			name:         "starts conversation when transcript is absent",
			useConfigDir: true,
			want:         []string{"--session-id", sessionID},
		},
		{
			name:         "reports unreadable transcript state",
			useConfigDir: true,
			configure: func(t *testing.T, configDir string) {
				t.Helper()

				if err := os.WriteFile(filepath.Join(configDir, "projects"), nil, 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			},
			wantErrContains: "read claude projects directory",
		},
		{
			name:         "resumes conversation when transcript is present",
			useConfigDir: true,
			configure:    writeTranscript([]byte("{}\n")),
			want:         []string{"--resume", sessionID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := ""
			if tt.useConfigDir {
				configDir = t.TempDir()
			}
			if tt.configure != nil {
				tt.configure(t, configDir)
			}
			got, err := claudeArguments(sessionID, options{
				configDir: configDir,
			})
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("claudeArguments() error = %v, want containing %q", err, tt.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("claudeArguments() error = %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("claudeArguments() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStartRequiresSessionID(t *testing.T) {
	if _, err := Start(t.Context(), ""); err == nil {
		t.Fatal("Start() error = nil, want session ID error")
	}
}

func setHarnessCommand(t *testing.T, command string) {
	t.Helper()

	original := claudeCommand
	claudeCommand = command
	t.Cleanup(func() {
		claudeCommand = original
	})
}

func fakeHarnessCommand(t *testing.T) string {
	t.Helper()

	command := filepath.Join(t.TempDir(), "fake-harness")
	if err := os.WriteFile(command, []byte(fakeHarnessScript), 0o755); err != nil {
		t.Fatalf("write fake harness command: %v", err)
	}

	return command
}

// envMap converts exec-style KEY=value entries into a map so environment tests
// can assert individual variables without repeated slice scans.
func envMap(env []string) map[string]string {
	values := map[string]string{}
	for _, value := range env {
		key, val, ok := strings.Cut(value, "=")
		if !ok {
			continue
		}
		values[key] = val
	}

	return values
}
