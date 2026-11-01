// Package local manages project metadata and files on the local filesystem.
package local

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

// Manager owns project files and borrows metadata storage. The server holds the
// workspace lock and stops all operations before closing the manager and store.
type Manager struct {
	root  *os.Root // workspace/projects
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
	workspace, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = workspace.Close() }()
	if err := workspace.MkdirAll("projects", 0o700); err != nil {
		return nil, err
	}
	if err := requireDirectory(workspace, "projects"); err != nil {
		return nil, err
	}
	root, err := workspace.OpenRoot("projects")
	if err != nil {
		return nil, err
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

// Create initializes a Git project with the given ID and display name. It returns
// after files and metadata are ready. Existing project directories are rejected.
func (m *Manager) Create(ctx context.Context, id, name string) (project.Project, error) {
	if err := ctx.Err(); err != nil {
		return project.Project{}, err
	}
	if err := project.ValidateID(id); err != nil {
		return project.Project{}, err
	}
	if err := project.ValidateName(name); err != nil {
		return project.Project{}, err
	}
	p := project.Project{
		ID: id, Name: strings.TrimSpace(name), Type: "git",
		State: project.StateCreating, Version: 1, CreatorToken: rand.Text(),
	}
	// Inserting first gives this attempt exclusive ownership of the ID.
	if err := m.store.InsertProject(ctx, p); err != nil {
		return project.Project{}, fmt.Errorf("create project %q: %w", p.ID, err)
	}
	cleanup, err := m.createFiles(ctx, p.ID)
	if err != nil {
		return project.Project{}, errors.Join(err, m.rollback(ctx, p, cleanup))
	}
	ready := p
	ready.State = project.StateReady
	ready, err = m.store.UpdateProject(ctx, p, ready)
	if err != nil {
		// The update may have committed. Leave files and metadata for manual cleanup.
		return project.Project{}, fmt.Errorf("record completed project: %w", err)
	}
	return ready, nil
}

// Remove only this attempt's files before releasing its registered ID.
func (m *Manager) rollback(ctx context.Context, p project.Project, cleanup func() error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	current, err := m.store.GetProject(ctx, p.ID)
	if err != nil {
		return err
	}
	if p.State != project.StateCreating || current != p {
		return store.ErrConflict
	}
	if err := cleanup(); err != nil {
		return fmt.Errorf("clean up project %q: %w", p.ID, err)
	}
	return m.store.DeleteProject(ctx, p)
}

// createFiles always returns cleanup, which removes only directories it created.
func (m *Manager) createFiles(ctx context.Context, id string) (func() error, error) {
	cleanup := func() error { return nil }
	if err := ctx.Err(); err != nil {
		return cleanup, err
	}
	if err := m.root.Mkdir(id, 0o700); err != nil {
		return cleanup, err
	}
	cleanup = func() error { return m.root.RemoveAll(id) }
	if err := requireDirectory(m.root, id); err != nil {
		return cleanup, err
	}
	repo := filepath.Join(id, "repo")
	if err := m.root.Mkdir(repo, 0o700); err != nil {
		return cleanup, err
	}
	path := filepath.Join(m.root.Name(), repo)
	return cleanup, initRepository(ctx, path)
}

func initRepository(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		"git",
		"init",
		"--quiet",
		"--initial-branch=main",
	)
	command.Dir = path
	command.WaitDelay = 2 * time.Second

	// Do not inherit GIT_DIR, templates, or injected developer configuration.
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git init: %w: %s", errors.Join(err, ctx.Err()), strings.TrimSpace(string(output)))
	}
	return nil
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
