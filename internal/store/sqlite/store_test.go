package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
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

func TestPersistedListing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ahh.db")
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	items, err := s.ListProjects(t.Context())
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty list: %v, %v", items, err)
	}
	_, err = s.db.ExecContext(t.Context(), `INSERT INTO projects
		(id,name,type,state,version,creator_token) VALUES
		('b','Same','git','failed',1,'attempt-b'),
		('a','Same','git',?,2,'attempt-a'),
		('c','Z','git','deleting',3,'attempt-c')`, project.StateReady)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	items, err = s.ListProjects(t.Context())
	if err != nil || len(items) != 3 {
		t.Fatalf("reopen list: %v, %v", items, err)
	}
	if items[0].ID != "a" || items[1].State != project.StateFailed || items[2].State != project.StateDeleting {
		t.Fatal(items)
	}
	p, err := s.GetProject(t.Context(), "a")
	if err != nil || p.Version != 2 || p.CreatorToken != "attempt-a" {
		t.Fatalf("get: %+v, %v", p, err)
	}
	if _, err := s.GetProject(t.Context(), "' OR 1=1 --"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProject(t.Context(), "a"); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Fatal("closed storage reported absence")
	}
}

func TestUnsupportedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ahh.db")
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(t.Context(), "PRAGMA user_version = 2"); err != nil {
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

func TestReadsRejectInvalidState(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "ahh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	// Simulate data written outside the application without schema validation.
	_, err = s.db.ExecContext(t.Context(), `PRAGMA ignore_check_constraints = ON;
		INSERT INTO projects (id, name, type, state, version)
		VALUES ('bad', 'Bad', 'git', 'unknown', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProject(t.Context(), "bad"); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Fatalf("invalid stored state did not cause a read failure: %v", err)
	}
	if _, err := s.ListProjects(t.Context()); err == nil {
		t.Fatal("listed a project with an invalid state")
	}
}
