// Package testutils provides storage checks that backend tests can reuse.
// Each helper requires empty storage; the caller owns setup and cleanup.
package testutils

import (
	"context"
	"errors"
	"testing"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

func TestDeleteProject(t *testing.T, s store.Store) {
	t.Helper()
	p := testProject()
	if err := s.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*project.Project)
	}{
		{name: "wrong ID", mutate: func(p *project.Project) { p.ID = "other" }},
		{name: "wrong type", mutate: func(p *project.Project) { p.Type = "other" }},
		{name: "wrong version", mutate: func(p *project.Project) { p.Version++ }},
		{name: "wrong state", mutate: func(p *project.Project) { p.State = project.StateReady }},
		{name: "wrong token", mutate: func(p *project.Project) { p.CreatorToken = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := p
			test.mutate(&before)
			if err := s.DeleteProject(t.Context(), before); !errors.Is(err, store.ErrConflict) {
				t.Fatalf("stale deletion: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.DeleteProject(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled deletion: %v", err)
	}
	if got, err := s.GetProject(t.Context(), p.ID); err != nil || got != p {
		t.Fatalf("rejected deletion changed metadata: %+v, %v", got, err)
	}
	if err := s.DeleteProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProject(t.Context(), p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted project still present: %v", err)
	}
	if got, err := s.ListProjects(t.Context()); err != nil || len(got) != 0 {
		t.Fatalf("deleted project still listed: %+v, %v", got, err)
	}
	if err := s.DeleteProject(t.Context(), p); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("repeated deletion: %v", err)
	}

	replacement := p
	replacement.CreatorToken = "new-attempt"
	if err := s.InsertProject(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(t.Context(), p); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("old attempt deleted replacement: %v", err)
	}
	ready := replacement
	ready.State = project.StateReady
	ready, err := s.UpdateProject(t.Context(), replacement, ready)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(t.Context(), ready); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("deleted a ready project: %v", err)
	}
	if got, err := s.GetProject(t.Context(), p.ID); err != nil || got != ready {
		t.Fatalf("ready project changed: %+v, %v", got, err)
	}
	deleting := ready
	deleting.State = project.StateDeleting
	deleting, err = s.UpdateProject(t.Context(), ready, deleting)
	if err != nil {
		t.Fatal(err)
	}
	stale := deleting
	stale.Version--
	if err := s.DeleteProject(t.Context(), stale); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale deleting version: %v", err)
	}
	stale = deleting
	stale.CreatorToken = p.CreatorToken
	if err := s.DeleteProject(t.Context(), stale); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale deleting token: %v", err)
	}
	if err := s.DeleteProject(t.Context(), deleting); err != nil {
		t.Fatal(err)
	}
}

func testProject() project.Project {
	return project.Project{
		ID: "project", Name: "Example", Type: "git",
		State: project.StateCreating, Version: 1, CreatorToken: "first",
	}
}

func TestListProjects(t *testing.T, s store.Store) {
	t.Helper()
	items, err := s.ListProjects(t.Context())
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty list: %v, %v", items, err)
	}
	for _, id := range []string{"b", "a", "c"} {
		p := testProject()
		p.ID = id
		if id == "c" {
			p.Name = "Alpha"
		}
		if err := s.InsertProject(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}
	items, err = s.ListProjects(t.Context())
	if err != nil || len(items) != 3 {
		t.Fatalf("list: %v, %v", items, err)
	}
	for i, id := range []string{"c", "a", "b"} {
		if items[i].ID != id {
			t.Fatalf("list not ordered by name then ID: %+v", items)
		}
	}
}

func TestGetProject(t *testing.T, s store.Store) {
	t.Helper()
	p := testProject()
	if err := s.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProject(t.Context(), p.ID)
	if err != nil || got != p {
		t.Fatalf("get: %+v, %v; want %+v", got, err, p)
	}
	got, err = s.GetProject(t.Context(), "' OR 1=1 --")
	if !errors.Is(err, store.ErrNotFound) || got != (project.Project{}) {
		t.Fatalf("missing project: %+v, %v", got, err)
	}
}

func TestInsertProject(t *testing.T, s store.Store) {
	t.Helper()
	p := testProject()
	if err := s.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	duplicate := p
	duplicate.Name = "Replacement"
	if err := s.InsertProject(t.Context(), duplicate); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate insert: %v", err)
	}
	got, err := s.GetProject(t.Context(), p.ID)
	if err != nil || got != p {
		t.Fatalf("duplicate insert changed metadata: %+v, %v", got, err)
	}
}

func TestUpdateProject(t *testing.T, s store.Store) {
	t.Helper()
	p := testProject()
	if err := s.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	after := p
	after.Name, after.State = "Renamed", project.StateFailed
	after.Error, after.CreatorToken = "initialization failed", "second"
	after.Version = 99 // The store must increment the persisted version instead.
	got, err := s.UpdateProject(t.Context(), p, after)
	after.Version = 2
	if err != nil || got != after {
		t.Fatalf("update: %+v, %v; want %+v", got, err, after)
	}
	saved, err := s.GetProject(t.Context(), p.ID)
	if err != nil || saved != after {
		t.Fatalf("persisted update: %+v, %v", saved, err)
	}
}

func TestUpdateProjectRejectsChanges(t *testing.T, s store.Store) {
	t.Helper()
	p := testProject()
	if err := s.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		mutate   func(before, after *project.Project)
		conflict bool
	}{
		{
			name: "stale version",
			mutate: func(before, _ *project.Project) {
				before.Version = 99
			},
			conflict: true,
		},
		{
			name: "wrong state",
			mutate: func(before, _ *project.Project) {
				before.State = project.StateReady
			},
			conflict: true,
		},
		{
			name: "wrong token",
			mutate: func(before, _ *project.Project) {
				before.CreatorToken = "other"
			},
			conflict: true,
		},
		{
			name: "changed ID",
			mutate: func(_, after *project.Project) {
				after.ID = "other"
			},
			conflict: true,
		},
		{
			name: "changed type",
			mutate: func(_, after *project.Project) {
				after.Type = "other"
			},
			conflict: true,
		},
		{
			name: "unset state",
			mutate: func(_, after *project.Project) {
				after.State = project.State{}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before, after := p, p
			test.mutate(&before, &after)
			got, err := s.UpdateProject(t.Context(), before, after)
			if err == nil || got != (project.Project{}) {
				t.Fatalf("rejected update: %+v, %v", got, err)
			}
			if test.conflict && !errors.Is(err, store.ErrConflict) {
				t.Fatalf("expected conflict, got %v", err)
			}
		})
	}
	got, err := s.GetProject(t.Context(), p.ID)
	if err != nil || got != p {
		t.Fatalf("rejected updates changed metadata: %+v, %v", got, err)
	}
}

// TestConcurrentUpdates needs two independent connections to the same storage.
func TestConcurrentUpdates(t *testing.T, a, b store.Store) {
	t.Helper()
	p := testProject()
	if err := a.InsertProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	after := p
	after.State = project.StateReady
	start, results := make(chan struct{}), make(chan error, 2)
	for _, s := range []store.Store{a, b} {
		go func() {
			<-start
			_, err := s.UpdateProject(t.Context(), p, after)
			results <- err
		}()
	}
	close(start)
	wins := 0
	for range 2 {
		if err := <-results; err == nil {
			wins++
		} else if !errors.Is(err, store.ErrConflict) {
			t.Error(err)
		}
	}
	if wins != 1 {
		t.Fatalf("successful concurrent updates: %d, want 1", wins)
	}
	after.Version = 2
	got, err := a.GetProject(t.Context(), p.ID)
	if err != nil || got != after {
		t.Fatalf("concurrent update: %+v, %v", got, err)
	}
}
