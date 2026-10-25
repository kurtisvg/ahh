// Package store defines metadata persistence contracts independently of backends.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/kurtisvg/ahh/internal/project"
)

var ErrNotFound = errors.New("record not found")

// Backend identifies a metadata storage implementation.
// Use ParseBackend to instantiate it. Its zero value is unset.
type Backend struct{ value string }

// SQLite is a predefined backend; callers must not reassign it.
var SQLite = Backend{value: "sqlite"}

func ParseBackend(value string) (Backend, error) {
	switch value {
	case "sqlite":
		return Backend{value: value}, nil
	default:
		return Backend{}, fmt.Errorf("unsupported store backend %q", value)
	}
}

func (b Backend) String() string { return b.value }

// Config selects a backend and its connection settings.
type Config struct {
	Backend Backend
	Path    string // SQLite database file; the caller owns its workspace lock.
}

// Store owns a metadata backend. Close after all users have finished.
// Reads may run concurrently and must return independent metadata values.
// GetProject returns ErrNotFound for a missing Project, not for storage failures.
type Store interface {
	ListProjects(context.Context) ([]project.Metadata, error)
	GetProject(context.Context, string) (project.Metadata, error)
	Close() error
}
