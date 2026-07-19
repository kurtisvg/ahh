package harness

import (
	"errors"
	"testing"
)

func TestParseType(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    Type
		wantErr bool
	}{
		{name: "Claude Code", value: "claude-code", want: ClaudeCode},
		{name: "empty", value: "", wantErr: true},
		{name: "unsupported", value: "codex", wantErr: true},
		{name: "surrounding whitespace", value: " claude-code ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseType(tt.value)
			if tt.wantErr {
				if !errors.Is(err, ErrUnsupportedType) {
					t.Fatalf("ParseType(%q) error = %v, want %v", tt.value, err, ErrUnsupportedType)
				}
				if got != "" {
					t.Errorf("ParseType(%q) = %q, want zero value", tt.value, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseType(%q) error = %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("ParseType(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
