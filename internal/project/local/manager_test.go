package local

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
	"github.com/kurtisvg/ahh/internal/store/sqlite"
)

func openTestManager(t *testing.T) (*Manager, *sqlite.Store, string) {
	t.Helper()
	path := t.TempDir()
	s, err := sqlite.Open(t.Context(), filepath.Join(path, "ahh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	m, err := New(t.Context(), path, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, s, path
}

func testProject(id string) project.Project {
	return project.Project{
		ID: id, Name: id, Type: "git",
		State: project.StateCreating, Version: 1, CreatorToken: "attempt",
	}
}

func TestManagerReads(t *testing.T) {
	m, s, workspace := openTestManager(t)
	p := testProject("my-project")
	if err := s.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if got, err := m.Get(t.Context(), p.ID); err != nil || got != p {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if got, err := m.List(t.Context()); err != nil || len(got) != 1 || got[0] != p {
		t.Fatalf("list: %+v, %v", got, err)
	}
	if _, err := m.Get(t.Context(), "unknown"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unregistered project: %v", err)
	}
	path := filepath.Join(workspace, "projects", p.ID, "repo")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("reading metadata changed files: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProject(t.Context(), p.ID); err != nil {
		t.Fatalf("manager closed borrowed store: %v", err)
	}
}

func TestNewPreservesIncompleteProject(t *testing.T) {
	m, s, workspace := openTestManager(t)
	p := testProject("my-project")
	if err := s.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(t.Context(), workspace, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got, err := reopened.Get(t.Context(), p.ID); err != nil || got != p {
		t.Fatalf("startup changed incomplete project: %+v, %v", got, err)
	}
}

func TestNewRejectsProjectsSymlink(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(workspace, "projects")); err != nil {
		t.Fatal(err)
	}
	_, s, _ := openTestManager(t)
	if m, err := New(t.Context(), workspace, s); err == nil {
		_ = m.Close()
		t.Fatal("accepted projects symlink")
	}
}
