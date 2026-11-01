package local

import (
	"context"
	"strings"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

// Rename changes the display name while preserving the project ID and files.
func (m *Manager) Rename(ctx context.Context, id, name string, version int64) (project.Project, error) {
	if err := project.ValidateName(name); err != nil {
		return project.Project{}, err
	}
	p, err := m.store.GetProject(ctx, id)
	if err != nil {
		return project.Project{}, err
	}
	if p.Version != version || p.State == project.StateCreating || p.State == project.StateDeleting {
		return project.Project{}, store.ErrConflict
	}
	next := p
	next.Name = strings.TrimSpace(name)
	return m.store.UpdateProject(ctx, p, next)
}
