package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
