package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

const projectColumns = "id, name, type, state, version, error, creator_token"

func (s *Store) GetProject(ctx context.Context, id string) (project.Metadata, error) {
	var p project.Metadata
	err := s.db.GetContext(ctx, &p, "SELECT "+projectColumns+" FROM projects WHERE id = ?", id)
	if errors.Is(err, sql.ErrNoRows) {
		return project.Metadata{}, store.ErrNotFound
	}
	if err != nil {
		return project.Metadata{}, err
	}
	return p, nil
}

func (s *Store) ListProjects(ctx context.Context) ([]project.Metadata, error) {
	projects := []project.Metadata{}
	err := s.db.SelectContext(ctx, &projects, "SELECT "+projectColumns+" FROM projects ORDER BY name, id")
	if err != nil {
		return nil, err
	}
	return projects, nil
}
