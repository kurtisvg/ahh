package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kurtisvg/ahh/internal/server/projects"
	"github.com/kurtisvg/ahh/internal/server/settings"
)

type projectAPIRunner struct{}

func (projectAPIRunner) Run(_ context.Context, _ []string, args ...string) ([]byte, error) {
	command := strings.Join(args, " ")
	switch {
	case strings.Contains(command, "remote get-url origin"):
		return []byte("git@github.com:owner/repository.git\n"), nil
	case strings.Contains(command, "ls-remote --symref origin HEAD"):
		return []byte("ref: refs/heads/main\tHEAD\nabc\tHEAD\n"), nil
	case strings.Contains(command, "for-each-ref"):
		return []byte("refs/heads/local\nrefs/remotes/origin/HEAD\nrefs/remotes/origin/main\n"), nil
	case strings.Contains(command, "rev-parse --is-bare-repository"):
		return []byte("true\n"), nil
	default:
		return nil, nil
	}
}

func TestServerProjectsAPI(t *testing.T) {
	stateDir := t.TempDir()
	settingsManager, err := settings.NewManager(stateDir)
	if err != nil {
		t.Fatalf("settings.NewManager() error = %v", err)
	}
	projectManager, err := projects.NewManager(
		stateDir,
		settingsManager,
		projects.WithCommandRunner(projectAPIRunner{}),
	)
	if err != nil {
		t.Fatalf("projects.NewManager() error = %v", err)
	}
	server := startTestServer(t, WithProjectManager(projectManager))
	defer shutdownTestServer(t, server)
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get("http://" + server.Addr + "/api/projects")
	if err != nil {
		t.Fatalf("GET /api/projects: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	var initial projectsResponse
	decodeJSON(t, resp, &initial)
	resp.Body.Close()
	if len(initial.Projects) != 0 {
		t.Fatalf("initial Projects = %d, want 0", len(initial.Projects))
	}

	requestBody := `{"name":"example-project","source":{"type":"github","repository":"owner/repository.git"}}`
	resp, err = client.Post("http://"+server.Addr+"/api/projects", "application/json", strings.NewReader(requestBody))
	if err != nil {
		t.Fatalf("POST /api/projects: %v", err)
	}
	assertStatus(t, resp, http.StatusCreated)
	var created projects.Metadata
	decodeJSON(t, resp, &created)
	resp.Body.Close()
	if created.ID != "example-project" || created.Name != "example-project" || created.Source.Repository != "owner/repository" || created.Status != projects.StatusReady {
		t.Fatalf("created Project = %+v", created)
	}

	resp, err = client.Get("http://" + server.Addr + "/api/projects/example-project")
	if err != nil {
		t.Fatalf("GET Project: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp, err = client.Get("http://" + server.Addr + "/api/projects/example-project/branches")
	if err != nil {
		t.Fatalf("GET Project branches: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	var branches branchesResponse
	decodeJSON(t, resp, &branches)
	resp.Body.Close()
	if len(branches.Branches) != 2 || branches.Branches[0].Kind != projects.BranchLocal {
		t.Fatalf("Project branches = %+v", branches.Branches)
	}

	patchBody, err := json.Marshal(updateProjectRequest{DefaultBranch: projects.Branch{Kind: projects.BranchLocal, Name: "local"}})
	if err != nil {
		t.Fatalf("marshal PATCH body: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch, "http://"+server.Addr+"/api/projects/example-project", bytes.NewReader(patchBody))
	if err != nil {
		t.Fatalf("new PATCH request: %v", err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("PATCH Project: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	var updated projects.Metadata
	decodeJSON(t, resp, &updated)
	resp.Body.Close()
	if updated.DefaultBranch.Kind != projects.BranchLocal {
		t.Fatalf("updated default branch = %+v, want local", updated.DefaultBranch)
	}

	req, err = http.NewRequestWithContext(t.Context(), http.MethodPatch, "http://"+server.Addr+"/api/projects/example-project", strings.NewReader(`{"name":"Renamed"}`))
	if err != nil {
		t.Fatalf("new immutable PATCH request: %v", err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("immutable PATCH Project: %v", err)
	}
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	resp, err = client.Post("http://"+server.Addr+"/api/projects/example-project/fetch", "application/json", nil)
	if err != nil {
		t.Fatalf("POST Project fetch: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	req, err = http.NewRequestWithContext(t.Context(), http.MethodDelete, "http://"+server.Addr+"/api/projects/example-project", nil)
	if err != nil {
		t.Fatalf("new DELETE request: %v", err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("DELETE Project: %v", err)
	}
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestServerCreateProjectValidation(t *testing.T) {
	server := startTestServer(t)
	defer shutdownTestServer(t, server)
	client := &http.Client{Timeout: 2 * time.Second}

	for _, body := range []string{
		`{"name":"","source":{"type":"github","repository":"owner/repo"}}`,
		`{"name":"not URL safe","source":{"type":"github","repository":"owner/repo"}}`,
		`{"name":"../escape","source":{"type":"github","repository":"owner/repo"}}`,
		`{"name":"Project","source":{"type":"gitlab","repository":"owner/repo"}}`,
		`{"name":"Project","source":{"type":"github","repository":"https://github.com/owner/repo"}}`,
		`{"name":"Project","source":{"type":"github","repository":"owner/repo"},"unknown":true}`,
	} {
		resp, err := client.Post("http://"+server.Addr+"/api/projects", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST invalid Project: %v", err)
		}
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	}
}
