package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kurtisvg/ahh/internal/project"
)

func TestOpenRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	s, err := Open(t.Context(), "ahh.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	projects, err := s.ListProjects(t.Context())
	if err != nil || projects == nil || len(projects) != 0 {
		t.Fatalf("relative database not initialized: %v, %v", projects, err)
	}
}

func TestMigrateDeletionFailureState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ahh.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = old.Close() })
	script, err := migrations.ReadFile("migrations/001.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.ExecContext(t.Context(), string(script)); err != nil {
		t.Fatal(err)
	}
	original := project.Project{
		ID: "existing", Name: "Existing", Type: "git",
		State: project.StateDeleting, Version: 4,
		Error: "prior error", CreatorToken: "original",
	}
	_, err = old.ExecContext(
		t.Context(),
		`
		INSERT INTO projects (id, name, type, state, version, error, creator_token)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		original.ID,
		original.Name,
		original.Type,
		original.State,
		original.Version,
		original.Error,
		original.CreatorToken,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if got, err := s.GetProject(t.Context(), original.ID); err != nil || got != original {
		t.Fatalf("migration changed metadata: %+v, %v", got, err)
	}
	failed := original
	failed.State = project.StateDeleteFailed
	failed, err = s.UpdateProject(t.Context(), original, failed)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got, err := reopened.GetProject(t.Context(), original.ID); err != nil || got != failed {
		t.Fatalf("delete_failed did not survive reopening: %+v, %v", got, err)
	}
}

func TestUnsupportedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ahh.db")
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), "PRAGMA user_version = 3"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if next, err := Open(t.Context(), path); err == nil {
		_ = next.Close()
		t.Fatal("accepted future schema")
	}
}
