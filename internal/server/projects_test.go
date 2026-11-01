package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

type testProjects struct{ err error }

func (p testProjects) Close() error { return nil }

func (p testProjects) ListProjects(context.Context) ([]project.Project, error) {
	return []project.Project{}, p.err
}
func (p testProjects) GetProject(context.Context, string) (project.Project, error) {
	return project.Project{}, p.err
}

func (p testProjects) InsertProject(context.Context, project.Project) error {
	return errors.New("unexpected InsertProject call")
}

func (p testProjects) DeleteProject(context.Context, project.Project) error {
	return errors.New("unexpected DeleteProject call")
}

func (p testProjects) UpdateProject(context.Context, project.Project, project.Project) (project.Project, error) {
	return project.Project{}, errors.New("unexpected UpdateProject call")
}

func TestProjectReads(t *testing.T) {
	for _, test := range []struct {
		path   string
		err    error
		status int
		body   string
	}{
		{"/api/projects", nil, 200, "[]"},
		{"/api/projects/missing", store.ErrNotFound, 404, "not_found"},
		{"/api/projects", errors.New("private database path"), 503, "storage_unavailable"},
	} {
		t.Run(test.body, func(t *testing.T) {
			s, err := startWithStore(t, t.Context(), Config{Address: "127.0.0.1"}, testProjects{err: test.err}, "unused")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Shutdown(t.Context()); _ = s.Wait() })
			response := httptest.NewRecorder()
			s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.body) {
				t.Fatalf("response: %d %s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "private database") {
				t.Fatal("exposed internal storage details")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cached project state")
			}
		})
	}
}

type mockCreationManager struct {
	project.Manager
	requestedID string
	err         error
}

func (p *mockCreationManager) Create(_ context.Context, id string) (project.Project, error) {
	p.requestedID = id
	return project.Project{ID: id, Name: id, Type: "git", State: project.StateReady}, p.err
}

func TestProjectCreationAPI(t *testing.T) {
	tests := []struct {
		name, body string
		err        error
		status     int
	}{
		{name: "created", body: `{"id":"alpha"}`, status: 201},
		{name: "display name is not an ID", body: `{"name":"alpha"}`, status: 400},
		{name: "empty", body: `{"id":""}`, status: 400},
		{name: "unsafe", body: `{"id":"../alpha"}`, status: 400},
		{name: "too long", body: `{"id":"` + strings.Repeat("a", 65) + `"}`, status: 400},
		{name: "unknown field", body: `{"id":"alpha","surprise":true}`, status: 400},
		{name: "trailing data", body: `{"id":"alpha"} {}`, status: 400},
		{name: "duplicate", body: `{"id":"alpha"}`, err: store.ErrConflict, status: 409},
		{name: "existing directory", body: `{"id":"alpha"}`, err: os.ErrExist, status: 409},
		{name: "setup failure", body: `{"id":"alpha"}`, err: errors.New("private path"), status: 503},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &mockCreationManager{err: test.err}
			s := &Server{projects: p}
			response := httptest.NewRecorder()
			s.createProject(response, httptest.NewRequest(
				http.MethodPost,
				"/api/projects",
				strings.NewReader(test.body),
			))
			if response.Code != test.status {
				t.Fatalf("%d: %s", response.Code, response.Body.String())
			}
			if test.status == 201 && p.requestedID != "alpha" {
				t.Fatalf("wrong creation ID: %q", p.requestedID)
			}
			if test.status == 400 && p.requestedID != "" {
				t.Fatal("dispatched invalid creation")
			}
			if strings.Contains(response.Body.String(), "private path") {
				t.Fatal("exposed internal error")
			}
		})
	}
}
