package project_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/project"
)

func TestStateRoundTrip(t *testing.T) {
	for _, tt := range []struct {
		value string
		state project.State
	}{
		{"creating", project.StateCreating},
		{"ready", project.StateReady},
		{"failed", project.StateFailed},
		{"deleting", project.StateDeleting},
		{"delete_failed", project.StateDeleteFailed},
	} {
		t.Run(tt.value, func(t *testing.T) {
			state, err := project.ParseState(tt.value)
			if err != nil || state != tt.state || state.String() != tt.value {
				t.Fatalf("parse = %v, %v", state, err)
			}
			data, err := json.Marshal(state)
			if err != nil || string(data) != `"`+tt.value+`"` {
				t.Fatalf("JSON = %s, %v", data, err)
			}
			var decoded project.State
			if err := json.Unmarshal(data, &decoded); err != nil || decoded != state {
				t.Fatalf("JSON round trip = %v, %v", decoded, err)
			}
			value, err := state.Value()
			if err != nil || value != tt.value {
				t.Fatalf("database value = %v, %v", value, err)
			}
			for _, input := range []any{value, []byte(tt.value)} {
				var scanned project.State
				if err := scanned.Scan(input); err != nil || scanned != state {
					t.Fatalf("database round trip = %v, %v", scanned, err)
				}
			}
		})
	}
}

func TestStateRejectsInvalidDatabaseValues(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
	}{
		{"null", nil},
		{"number", int64(1)},
		{"empty", ""},
		{"unknown", "unknown"},
		{"unknown bytes", []byte("unknown")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := project.StateReady
			if err := state.Scan(tt.value); err == nil || state != project.StateReady {
				t.Fatalf("invalid database value changed state: %v, %v", state, err)
			}
		})
	}
	if _, err := (project.State{}).Value(); err == nil {
		t.Fatal("accepted unset state for persistence")
	}
}

func TestStateRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "unknown", "READY", " ready "} {
		t.Run("text/"+value, func(t *testing.T) {
			if _, err := project.ParseState(value); err == nil {
				t.Fatal("accepted invalid state")
			}
			state := project.StateReady
			if err := state.UnmarshalText([]byte(value)); err == nil || state != project.StateReady {
				t.Fatalf("invalid text changed state: %v, %v", state, err)
			}
		})
	}
	for _, data := range []string{`""`, `"unknown"`, `null`, `1`, `{}`, `[]`, `true`} {
		t.Run("JSON/"+data, func(t *testing.T) {
			state := project.StateReady
			if err := json.Unmarshal([]byte(data), &state); err == nil || state != project.StateReady {
				t.Fatalf("invalid JSON changed state: %v, %v", state, err)
			}
		})
	}
	if _, err := json.Marshal(project.State{}); err == nil {
		t.Fatal("serialized unset state")
	}
}

func TestValidateID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		valid bool
	}{
		{name: "name", id: "my-project", valid: true},
		{name: "digits", id: "project-123", valid: true},
		{name: "single character", id: "a", valid: true},
		{name: "maximum length", id: strings.Repeat("a", 64), valid: true},
		{name: "too long", id: strings.Repeat("a", 65)},
		{name: "empty", id: ""},
		{name: "uppercase", id: "My-Project"},
		{name: "spaces", id: "my project"},
		{name: "leading hyphen", id: "-project"},
		{name: "trailing hyphen", id: "project-"},
		{name: "repeated hyphen", id: "my--project"},
		{name: "traversal", id: "../project"},
		{name: "nested path", id: "my/project"},
		{name: "backslash", id: `my\project`},
		{name: "encoded separator", id: "my%2fproject"},
		{name: "query", id: "project?x=1"},
		{name: "fragment", id: "project#section"},
		{name: "unicode", id: "café"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := project.ValidateID(test.id); (err == nil) != test.valid {
				t.Fatalf("ValidateID(%q) = %v", test.id, err)
			}
		})
	}
}
