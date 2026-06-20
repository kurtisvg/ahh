package cmd

import (
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/version"
)

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
