// Package local manages project metadata and files on the local filesystem.
package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

// Manager owns project files and borrows metadata storage. The server holds the
// workspace lock and stops all operations before closing the manager and store.
type Manager struct {
	root  *os.Root
	store store.Store
}

// New opens a manager for the workspace, creating projects/ if needed.
func New(ctx context.Context, path string, storage store.Store) (*Manager, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("workspace path is required")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	if err := root.MkdirAll("projects", 0o700); err != nil {
		return nil, errors.Join(err, root.Close())
	}
	if err := requireDirectory(root, "projects"); err != nil {
		return nil, errors.Join(err, root.Close())
	}
	return &Manager{root: root, store: storage}, nil
}

// Close releases the root handle after operations stop, leaving the store open.
func (m *Manager) Close() error { return m.root.Close() }

// List returns all registered projects.
func (m *Manager) List(ctx context.Context) ([]project.Project, error) {
	return m.store.ListProjects(ctx)
}

// Get returns the registered project with the given ID, without checking its files.
func (m *Manager) Get(ctx context.Context, id string) (project.Project, error) {
	return m.store.GetProject(ctx, id)
}

// requireDirectory rejects symlinks, including aliases within the workspace.
func requireDirectory(root *os.Root, path string) error {
	info, err := root.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}
