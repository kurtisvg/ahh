// Package project defines project data and management operations.
package project

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Manager provides project operations independently of their backend.
type Manager interface {
	// Create initializes a Git project with the given ID and registers it.
	Create(context.Context, string) (Project, error)

	// List returns all registered projects.
	List(context.Context) ([]Project, error)

	// Get returns the registered project with the given ID.
	Get(context.Context, string) (Project, error)

	// Rename updates the display name if the stored version still matches.
	Rename(context.Context, string, string, int64) (Project, error)

	// Close releases resources after project operations have stopped.
	Close() error
}

var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateName checks a project's display name, independently of its stable ID.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return fmt.Errorf("use a name of 1–100 characters without control characters")
	}
	return nil
}

// ValidateID checks a project name for use as one URL segment and directory name.
func ValidateID(id string) error {
	if len(id) > 64 || !idPattern.MatchString(id) {
		return fmt.Errorf(
			"invalid project ID %q: use 1-64 lowercase letters or digits, separated by single hyphens",
			id,
		)
	}
	return nil
}

// Project describes a project's identity, settings, and lifecycle.
type Project struct {
	// Immutable after creation.
	ID   string `json:"id"`   // User-chosen URL-safe project name.
	Type string `json:"type"` // Repository kind; currently "git".

	// Mutable settings and lifecycle.
	Name  string `json:"name"`
	State State  `json:"state"`
	Error string `json:"error,omitempty"` // User-facing preparation or deletion failure.
	// CreatorToken identifies the current repository preparation attempt.
	CreatorToken string `json:"-" db:"creator_token"`

	// Version is incremented by the store on each update to reject stale writes.
	Version int64 `json:"version"`
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
