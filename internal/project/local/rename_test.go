package local

import (
	"errors"
	"testing"

	"github.com/kurtisvg/ahh/internal/store"
)

func TestRenamePreservesIdentityAndRejectsStaleWrites(t *testing.T) {
	m, _, _ := openTestManager(t)
	p, err := m.Create(t.Context(), "my-project")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := m.Rename(t.Context(), p.ID, " New name ", p.Version)
	if err != nil || renamed.Name != "New name" || renamed.ID != p.ID || renamed.Version != p.Version+1 {
		t.Fatalf("rename: %+v, %v", renamed, err)
	}
	if _, err := m.Rename(t.Context(), p.ID, "Stale", p.Version); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale rename: %v", err)
	}
	if _, err := m.Rename(t.Context(), p.ID, " ", renamed.Version); err == nil {
		t.Fatal("accepted empty name")
	}
	if got, err := m.Get(t.Context(), p.ID); err != nil || got != renamed {
		t.Fatalf("changed stored project: %+v, %v", got, err)
	}
}
