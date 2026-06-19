package cmd

import "testing"

func TestRunCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantPath string
		wantHost string
		wantPort string
		wantErr  bool
	}{
		{
			name:     "claude code",
			args:     []string{"run", "claude-code"},
			wantPath: "ahh run",
			wantHost: "127.0.0.1",
			wantPort: "18081",
		},
		{
			name:     "configured address",
			args:     []string{"run", "claude-code", "--host", "localhost", "--port", "0"},
			wantPath: "ahh run",
			wantHost: "localhost",
			wantPort: "0",
		},
		{
			name:    "missing harness",
			args:    []string{"run"},
			wantErr: true,
		},
		{
			name:    "unsupported harness",
			args:    []string{"run", "codex"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd, _, err := invokeCommandWithoutRun(t, tt.args)
			if tt.wantErr && err == nil {
				t.Fatal("error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatal(err)
			}
			if tt.wantPath != "" && cmd.CommandPath() != tt.wantPath {
				got := cmd.CommandPath()
				t.Fatalf("command path = %q, want %q", got, tt.wantPath)
			}
			if tt.wantHost != "" && flagString(t, cmd, "host") != tt.wantHost {
				got := flagString(t, cmd, "host")
				t.Fatalf("host = %q, want %q", got, tt.wantHost)
			}
			if tt.wantPort != "" && flagString(t, cmd, "port") != tt.wantPort {
				got := flagString(t, cmd, "port")
				t.Fatalf("port = %q, want %q", got, tt.wantPort)
			}
		})
	}
}
