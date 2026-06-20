package cmd

import "testing"

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
