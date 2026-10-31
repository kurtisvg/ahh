// Package store defines metadata persistence contracts independently of backends.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/kurtisvg/ahh/internal/project"
)

var ErrNotFound = errors.New("record not found")
var ErrConflict = errors.New("record changed or operation already in progress")

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
// Operations may run concurrently. Reads return independent metadata values.
// GetProject returns ErrNotFound for a missing Project, not for storage failures.
type Store interface {
	ListProjects(context.Context) ([]project.Project, error)
	GetProject(context.Context, string) (project.Project, error)

	// InsertProject inserts creating metadata with version 1 and a CreatorToken.
	// An existing ID or invalid state, version, or token returns ErrConflict.
	InsertProject(context.Context, project.Project) error

	// UpdateProject atomically matches the prior ID, version, state, and
	// CreatorToken, then applies changes and increments the stored version.
	// The supplied after.Version is ignored.
	// ID and Type cannot change. A mismatch returns ErrConflict without writing.
	UpdateProject(ctx context.Context, before, after project.Project) (project.Project, error)

	// DeleteProject removes an unchanged creating or deleting record after cleanup.
	// It matches ID, Type, Version, State, and CreatorToken; mismatches return ErrConflict.
	DeleteProject(context.Context, project.Project) error

	Close() error
}
