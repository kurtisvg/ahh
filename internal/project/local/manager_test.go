package local

import (
	"context"
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

func TestManagerCreatesProject(t *testing.T) {
	m, s, workspace := openTestManager(t)
	outside := t.TempDir()
	t.Setenv("GIT_DIR", outside)
	p, err := m.Create(t.Context(), "my-project", " My Project ")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "my-project" || p.Name != "My Project" || p.Type != "git" ||
		p.State != project.StateReady || p.Version != 2 {
		t.Fatalf("created metadata: %+v", p)
	}
	wantPath := filepath.Join(workspace, "projects", p.ID, "repo")
	if p.CreatorToken == "" {
		t.Fatalf("created storage: %+v", p)
	}
	if got, err := s.GetProject(t.Context(), p.ID); err != nil || got != p {
		t.Fatalf("saved metadata: %+v, %v", got, err)
	}
	head, err := os.ReadFile(filepath.Join(wantPath, ".git", "HEAD"))
	if err != nil || string(head) != "ref: refs/heads/main\n" {
		t.Fatalf("HEAD: %s, %v", head, err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("inherited GIT_DIR: %v, %v", entries, err)
	}
	work := filepath.Join(wantPath, "work.txt")
	if err := os.WriteFile(work, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(t.Context(), p.ID, "My Project"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	if data, err := os.ReadFile(work); err != nil || string(data) != "keep" {
		t.Fatalf("duplicate creation changed files: %q, %v", data, err)
	}

}

func TestManagerCreationFailureReleasesName(t *testing.T) {
	m, s, workspace := openTestManager(t)
	id := "my-project"
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	if got, err := m.Create(t.Context(), id, "My Project"); err == nil || got != (project.Project{}) {
		t.Fatalf("failed creation: %+v, %v", got, err)
	}
	if _, err := s.GetProject(t.Context(), id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed creation retained metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "projects", id)); !os.IsNotExist(err) {
		t.Fatalf("failed creation retained files: %v", err)
	}
	t.Setenv("PATH", originalPath)
	if _, err := m.Create(t.Context(), id, "My Project"); err != nil {
		t.Fatalf("fresh creation could not reuse name: %v", err)
	}
}

type cancelAfterInsert struct {
	store.Store
	cancel context.CancelFunc
}

func (s cancelAfterInsert) InsertProject(ctx context.Context, p project.Project) error {
	err := s.Store.InsertProject(ctx, p)
	s.cancel()
	return err
}

func TestCanceledCreationCleansMetadata(t *testing.T) {
	m, s, _ := openTestManager(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.store = cancelAfterInsert{Store: s, cancel: cancel}
	id := "my-project"
	if _, err := m.Create(ctx, id, "My Project"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled creation: %v", err)
	}
	if _, err := s.GetProject(t.Context(), id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cancellation skipped cleanup: %v", err)
	}
}

type failingCompletion struct{ store.Store }

func (s failingCompletion) UpdateProject(
	context.Context, project.Project, project.Project,
) (project.Project, error) {
	return project.Project{}, errors.New("database unavailable")
}

func TestUncertainCompletionPreservesTrackedFiles(t *testing.T) {
	m, s, workspace := openTestManager(t)
	m.store = failingCompletion{Store: s}
	id := "my-project"
	if got, err := m.Create(t.Context(), id, "My Project"); err == nil || got != (project.Project{}) {
		t.Fatalf("unconfirmed completion: %+v, %v", got, err)
	}
	saved, err := s.GetProject(t.Context(), id)
	if err != nil || saved.State != project.StateCreating {
		t.Fatalf("lost pending metadata: %+v, %v", saved, err)
	}
	head := filepath.Join(workspace, "projects", id, "repo", ".git", "HEAD")
	if _, err := os.Stat(head); err != nil {
		t.Fatalf("lost complete files: %v", err)
	}
}

func TestConcurrentProjectCreation(t *testing.T) {
	m, _, _ := openTestManager(t)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := m.Create(t.Context(), "same-name", "My Project")
			results <- err
		}()
	}
	wins := 0
	for range 2 {
		if err := <-results; err == nil {
			wins++
		} else if !errors.Is(err, store.ErrConflict) {
			t.Errorf("unexpected creation failure: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("successful creations = %d, want 1", wins)
	}
}

func TestCleanupFailureRetainsCreationRecord(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	m, s, workspace := openTestManager(t)
	id := "my-project"
	path := filepath.Join(workspace, "projects", id, "repo")
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf partial > partial.txt\n/bin/chmod 000 .\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
	if _, err := m.Create(t.Context(), id, "My Project"); err == nil {
		t.Fatal("creation unexpectedly succeeded")
	}
	if got, err := s.GetProject(t.Context(), id); err != nil || got.State != project.StateCreating {
		t.Fatalf("cleanup failure lost ownership: %+v, %v", got, err)
	}
}

func TestCreationValidatesBeforeInserting(t *testing.T) {
	tests := []struct {
		name, id, displayName string
	}{
		{name: "unsafe ID", id: "../escape", displayName: "My Project"},
		{name: "empty ID", id: "", displayName: "My Project"},
		{name: "empty display name", id: "my-project", displayName: " "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m, s, _ := openTestManager(t)
			if _, err := m.Create(t.Context(), test.id, test.displayName); err == nil {
				t.Fatal("accepted invalid project")
			}
			if got, err := s.ListProjects(t.Context()); err != nil || len(got) != 0 {
				t.Fatalf("invalid request inserted metadata: %+v, %v", got, err)
			}
		})
	}
}

func TestCreationPreservesExistingFiles(t *testing.T) {
	tests := []struct {
		name  string
		setup func(string, string) error
	}{
		{name: "nonempty directory", setup: func(path, outside string) error {
			if err := os.Mkdir(path, 0o700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(path, "keep"), []byte("keep"), 0o600)
		}},
		{name: "file", setup: func(path, outside string) error {
			return os.WriteFile(path, []byte("keep"), 0o600)
		}},
		{name: "symlink", setup: func(path, outside string) error {
			return os.Symlink(outside, path)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m, s, workspace := openTestManager(t)
			id := "my-project"
			path := filepath.Join(workspace, "projects", id)
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := test.setup(path, outside); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.Create(t.Context(), id, "My Project"); err == nil {
				t.Fatal("accepted occupied project path")
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("replaced existing path: %v", err)
			}
			marker := path
			if !before.Mode().IsRegular() {
				marker = filepath.Join(path, "keep")
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
				t.Fatalf("changed existing contents: %q, %v", data, err)
			}
			if _, err := s.GetProject(t.Context(), id); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("rejected directory retained metadata: %v", err)
			}
		})
	}
}

func TestCreationRejectsExistingEmptyDirectory(t *testing.T) {
	m, s, workspace := openTestManager(t)
	id := "my-project"
	path := filepath.Join(workspace, "projects", id)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(t.Context(), id, "My Project"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing directory: %v", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("existing directory changed: %v, %v", entries, err)
	}
	if _, err := s.GetProject(t.Context(), id); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("rejected directory retained metadata: %v", err)
	}
}

func TestCleanupUsesOpenedProjectsDirectory(t *testing.T) {
	m, _, workspace := openTestManager(t)
	cleanup, err := m.createFiles(t.Context(), "my-project")
	if err != nil {
		t.Fatal(err)
	}
	projects := filepath.Join(workspace, "projects")
	saved := filepath.Join(workspace, "saved-projects")
	if err := os.Rename(projects, saved); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(projects, "my-project")
	if err := os.MkdirAll(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(replacement, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(saved, "my-project")); !os.IsNotExist(err) {
		t.Fatalf("original project was not cleaned up: %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("cleanup touched replacement directory: %q, %v", data, err)
	}
}
