package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

func (s *Store) GetProject(ctx context.Context, id string) (project.Metadata, error) {
	query := `
		SELECT *
		FROM projects
		WHERE id = ?`

	var p project.Metadata
	err := s.db.GetContext(ctx, &p, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return project.Metadata{}, store.ErrNotFound
	}
	if err != nil {
		return project.Metadata{}, err
	}

	return p, nil
}

func (s *Store) ListProjects(ctx context.Context) ([]project.Metadata, error) {
	query := `
		SELECT *
		FROM projects
		ORDER BY name, id`

	projects := []project.Metadata{}
	err := s.db.SelectContext(ctx, &projects, query)
	if err != nil {
		return nil, err
	}

	return projects, nil
}

func (s *Store) InsertProject(ctx context.Context, p project.Metadata) error {
	if p.State != project.StateCreating || p.Version != 1 || p.CreatorToken == "" {
		return store.ErrConflict
	}

	query := `
		INSERT INTO projects (
			id, name, type, state, version, error, creator_token
		)
		VALUES (
			:id, :name, :type, :state, :version, :error, :creator_token
		)
		ON CONFLICT (id) DO NOTHING`

	result, err := s.db.NamedExecContext(ctx, query, p)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return store.ErrConflict
	}

	return nil
}

func (s *Store) UpdateProject(ctx context.Context, before, after project.Metadata) (project.Metadata, error) {
	if after.ID != before.ID || after.Type != before.Type {
		return project.Metadata{}, store.ErrConflict
	}

	params := struct {
		Before, After project.Metadata
	}{
		Before: before,
		After:  after,
	}

	query, args, err := s.db.BindNamed(`
		UPDATE projects
		SET
			name = :after.name,
			state = :after.state,
			version = version + 1,
			error = :after.error,
			creator_token = :after.creator_token
		WHERE id = :before.id
			AND version = :before.version
			AND state = :before.state
			AND creator_token = :before.creator_token
		RETURNING *`, params)
	if err != nil {
		return project.Metadata{}, err
	}

	var p project.Metadata
	err = s.db.GetContext(ctx, &p, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return project.Metadata{}, store.ErrConflict
	}
	if err != nil {
		return project.Metadata{}, err
	}

	return p, nil
}
