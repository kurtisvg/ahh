package sqlite

import (
	"path/filepath"
	"testing"
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
