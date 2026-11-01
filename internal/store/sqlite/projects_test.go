package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
	"github.com/kurtisvg/ahh/internal/store/testutils"
)

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func testProject() project.Project {
	return project.Project{
		ID: "project", Name: "Example", Type: "git",
		State: project.StateCreating, Version: 1, CreatorToken: "first",
	}
}

func TestListProjects(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "ahh.db"))
	testutils.TestListProjects(t, s)
}

func TestGetProject(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "ahh.db"))
	testutils.TestGetProject(t, s)
}

func TestInsertProject(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "ahh.db"))
	testutils.TestInsertProject(t, s)
}

func TestUpdateProject(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "ahh.db"))
	testutils.TestUpdateProject(t, s)
}

func TestUpdateProjectRejectsChanges(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "ahh.db"))
	testutils.TestUpdateProjectRejectsChanges(t, s)
}

func TestUpdateProjectConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ahh.db")
	a, b := openTestStore(t, path), openTestStore(t, path)
	testutils.TestConcurrentUpdates(t, a, b)
}

func TestProjectPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ahh.db")
	s := openTestStore(t, path)
	p := testProject()
	if err := s.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTestStore(t, path)
	got, err := s.GetProject(t.Context(), p.ID)
	if err != nil || got != p {
		t.Fatalf("reopened project: %+v, %v", got, err)
	}
	items, err := s.ListProjects(t.Context())
	if err != nil || len(items) != 1 || items[0] != p {
		t.Fatalf("reopened list: %+v, %v", items, err)
	}
}

func TestProjectStorageFailures(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "ahh.db"))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	p := testProject()
	for name, operation := range map[string]func() error{
		"list":   func() error { _, err := s.ListProjects(t.Context()); return err },
		"get":    func() error { _, err := s.GetProject(t.Context(), p.ID); return err },
		"insert": func() error { return s.InsertProject(t.Context(), p) },
		"update": func() error { _, err := s.UpdateProject(t.Context(), p, p); return err },
	} {
		t.Run(name, func(t *testing.T) {
			err := operation()
			if err == nil || errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotFound) {
				t.Fatalf("closed storage reported absence or conflict: %v", err)
			}
		})
	}
}

func TestReadsRejectInvalidState(t *testing.T) {
	s := openTestStore(t, filepath.Join(t.TempDir(), "ahh.db"))
	// Simulate data written outside the application without schema validation.
	_, err := s.db.ExecContext(t.Context(), `PRAGMA ignore_check_constraints = ON;
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
