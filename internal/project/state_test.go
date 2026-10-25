package project_test

import (
	"encoding/json"
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
