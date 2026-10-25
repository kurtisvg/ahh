// Package project defines Project metadata and lifecycle states.
package project

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Metadata describes a Project's identity and lifecycle.
type Metadata struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"` // Repository kind; currently "git".
	State   State  `json:"state"`
	Version int64  `json:"version"`         // Increases on updates to reject stale writes.
	Error   string `json:"error,omitempty"` // User-facing preparation or deletion failure.

	// CreatorToken identifies the current repository preparation attempt.
	CreatorToken string `json:"-" db:"creator_token"`
}

// State describes a Project's repository lifecycle.
// You must use ParseState to instantiate it.
type State struct{ value string }

// Predefined states are shared values; callers must not reassign them.
var (
	StateCreating = State{value: "creating"}
	StateReady    = State{value: "ready"}
	StateFailed   = State{value: "failed"}
	StateDeleting = State{value: "deleting"}
)

// ParseState accepts only recognized lifecycle states, with exact spelling.
func ParseState(value string) (State, error) {
	switch value {
	case "creating", "ready", "failed", "deleting":
		return State{value: value}, nil
	default:
		return State{}, fmt.Errorf("invalid project state %q", value)
	}
}

func (s State) String() string { return s.value }

func (s State) MarshalText() ([]byte, error) {
	if _, err := ParseState(s.value); err != nil {
		return nil, err
	}
	return []byte(s.value), nil
}

func (s *State) UnmarshalText(text []byte) error {
	parsed, err := ParseState(string(text))
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// UnmarshalJSON also rejects null, which would bypass UnmarshalText.
func (s *State) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return s.UnmarshalText([]byte(value))
}

// Scan implements sql.Scanner.
func (s *State) Scan(value any) error {
	switch value := value.(type) {
	case string:
		return s.UnmarshalText([]byte(value))
	case []byte:
		return s.UnmarshalText(value)
	default:
		return fmt.Errorf("cannot scan project state from %T", value)
	}
}

// Value implements driver.Valuer.
func (s State) Value() (driver.Value, error) {
	if _, err := ParseState(s.value); err != nil {
		return nil, err
	}
	return s.value, nil
}
