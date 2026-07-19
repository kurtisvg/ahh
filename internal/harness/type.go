package harness

import (
	"errors"
	"fmt"
)

// Type identifies a harness implementation.
type Type string

// TypeClaudeCode identifies the Claude Code harness.
const TypeClaudeCode Type = "claude-code"

// ErrUnsupportedType indicates that a string does not identify a supported harness.
var ErrUnsupportedType = errors.New("unsupported harness type")

// ParseType validates and returns the harness type identified by value.
func ParseType(value string) (Type, error) {
	harnessType := Type(value)
	switch harnessType {
	case TypeClaudeCode:
		return harnessType, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedType, value)
	}
}
