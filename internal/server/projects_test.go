package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	requestedID   string
	requestedName string
	err           error
}

func (p *mockCreationManager) Create(_ context.Context, id, name string) (project.Project, error) {
	p.requestedID = id
	p.requestedName = name
	return project.Project{ID: id, Name: name, Type: "git", State: project.StateReady}, p.err
}

func TestProjectCreationAPI(t *testing.T) {
	tests := []struct {
		name, body string
		err        error
		status     int
	}{
		{
			name:   "created",
			body:   `{"name":"My Project","id":"alpha"}`,
			status: 201,
		},
		{
			name:   "display name is not an ID",
			body:   `{"name":"alpha"}`,
			status: 400,
		},
		{
			name:   "missing display name",
			body:   `{"id":"alpha"}`,
			status: 400,
		},
		{
			name:   "empty display name",
			body:   `{"id":"alpha","name":" "}`,
			status: 400,
		},
		{
			name:   "empty",
			body:   `{"name":"My Project","id":""}`,
			status: 400,
		},
		{
			name:   "unsafe",
			body:   `{"name":"My Project","id":"../alpha"}`,
			status: 400,
		},
		{
			name:   "too long",
			body:   `{"name":"My Project","id":"` + strings.Repeat("a", 65) + `"}`,
			status: 400,
		},
		{
			name:   "unknown field",
			body:   `{"name":"My Project","id":"alpha","surprise":true}`,
			status: 400,
		},
		{
			name:   "trailing data",
			body:   `{"name":"My Project","id":"alpha"} {}`,
			status: 400,
		},
		{
			name:   "duplicate",
			body:   `{"name":"My Project","id":"alpha"}`,
			err:    store.ErrConflict,
			status: 409,
		},
		{
			name:   "existing directory",
			body:   `{"name":"My Project","id":"alpha"}`,
			err:    os.ErrExist,
			status: 409,
		},
		{
			name:   "setup failure",
			body:   `{"name":"My Project","id":"alpha"}`,
			err:    errors.New("private path"),
			status: 503,
		},
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
			if test.status == 201 && (p.requestedID != "alpha" || p.requestedName != "My Project") {
				t.Fatalf("wrong creation input: ID %q, name %q", p.requestedID, p.requestedName)
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

func TestConfirmedDeletionExcludesLaunch(t *testing.T) {
	s, projects, workspace := setupProjectServer(t)
	p := readyProject(t, projects, "alpha")
	path := "/api/projects/" + p.ID
	request := func(method, path string) int {
		response := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		return response.Code
	}
	if code := request(http.MethodDelete, path); code != http.StatusBadRequest {
		t.Fatal(code)
	}
	repo := filepath.Join(workspace, "projects", p.ID, "repo")
	if _, err := os.Stat(repo); err != nil {
		t.Fatal(err)
	}
	terminal := path + "/conversations/default/terminal"
	if code := request(http.MethodPost, terminal); code != http.StatusAccepted {
		t.Fatal(code)
	}
	status := waitForProjectTerminal(t, s, terminal)
	child, err := s.executions.Process(projectExecutionKey(p), status.Invocation)
	if err != nil {
		t.Fatal(err)
	}
	var calls sync.WaitGroup
	for range 4 {
		calls.Go(func() {
			code := request(http.MethodPost, terminal)
			switch code {
			case http.StatusAccepted, http.StatusConflict, http.StatusNotFound, http.StatusServiceUnavailable:
			default:
				t.Errorf("start returned %d", code)
			}
		})
	}
	if code := request(http.MethodDelete, path+"?discard=true"); code != http.StatusNoContent {
		t.Fatal(code)
	}
	calls.Wait()
	if _, err := projects.Get(t.Context(), p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("deletion did not finish", err)
	}
	select {
	case <-child.done:
	default:
		t.Fatal("writer survived deletion")
	}
	if _, err := os.Stat(repo); !os.IsNotExist(err) {
		t.Fatalf("files remain: %v", err)
	}
	if code := request(http.MethodDelete, path+"?discard=true"); code != http.StatusNoContent {
		t.Fatal(code)
	}

	replacement := readyProject(t, projects, p.ID)
	if projectExecutionKey(replacement) == projectExecutionKey(p) {
		t.Fatal("reused previous project's execution identity")
	}
	if code := request(http.MethodPost, terminal); code != http.StatusAccepted {
		t.Fatal(code)
	}
	waitForProjectTerminal(t, s, terminal)
	if code := request(http.MethodGet, terminal+"/"+status.Invocation+"/pty"); code != http.StatusConflict {
		t.Fatal(code)
	}
}

func TestDeletionPreservesUnconfirmedWriter(t *testing.T) {
	s, projects, workspace := setupProjectServer(t)
	p := readyProject(t, projects, "uncertain")
	done := make(chan struct{})
	close(done)
	s.executions = newExecutionManager(func(context.Context, string) (*wrapperProcess, error) {
		return &wrapperProcess{done: done, cancel: func() {}}, nil
	})
	if err := s.executions.startAndWait(t.Context(), projectExecutionKey(p), ""); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	path := "/api/projects/" + p.ID
	s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, path+"?discard=true", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal(response.Code)
	}
	got, err := projects.Get(t.Context(), p.ID)
	if err != nil || got != p {
		t.Fatalf("project changed before confirmed shutdown: %+v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "projects", p.ID, "repo", ".git")); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, path+"/conversations/default/terminal", nil),
	)
	var status processStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.StartAllowed == nil || *status.StartAllowed {
		t.Fatal("start allowed with unconfirmed writer")
	}
}

type delayedProjectRead struct {
	project.Manager
	read    chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (m *delayedProjectRead) Get(ctx context.Context, id string) (project.Project, error) {
	p, err := m.Manager.Get(ctx, id)
	if m.calls.Add(1) == 1 {
		close(m.read)
		<-m.release
	}
	return p, err
}

func TestDeletionWaitsForStartAdmission(t *testing.T) {
	s, projects, _ := setupProjectServer(t)
	p := readyProject(t, projects, "delayed")
	delayed := &delayedProjectRead{
		Manager: projects,
		read:    make(chan struct{}),
		release: make(chan struct{}),
	}
	s.projects = delayed
	s.executions = newExecutionManager(func(ctx context.Context, _ string) (*wrapperProcess, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	path := "/api/projects/" + p.ID
	start := httptest.NewRecorder()
	deletion := httptest.NewRecorder()
	var requests sync.WaitGroup
	t.Cleanup(func() {
		close(delayed.release)
		requests.Wait()
	})
	requests.Go(func() {
		s.http.Handler.ServeHTTP(start, httptest.NewRequest(
			http.MethodPost, path+"/conversations/default/terminal", nil,
		))
	})
	select {
	case <-delayed.read:
	case <-time.After(5 * time.Second):
		t.Fatal("start did not read the project")
	}
	requests.Go(func() {
		s.http.Handler.ServeHTTP(deletion, httptest.NewRequest(
			http.MethodDelete, path+"?discard=true", nil,
		))
	})
	delayed.release <- struct{}{}
	requests.Wait()
	if start.Code != http.StatusAccepted || deletion.Code != http.StatusNoContent {
		t.Fatalf("start: %d, delete: %d", start.Code, deletion.Code)
	}
	if status := s.executions.Status(projectExecutionKey(p)); status.State != "stopped" {
		t.Fatalf("execution survived deletion: %+v", status)
	}
}

func TestDeletionWaitsForPendingStartup(t *testing.T) {
	s, projects, workspace := setupProjectServer(t)
	p := readyProject(t, projects, "pending")
	other := readyProject(t, projects, "other")
	entered := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	s.executions = newExecutionManager(func(ctx context.Context, _ string) (*wrapperProcess, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return nil, ctx.Err()
	})
	path := "/api/projects/" + p.ID
	start := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(start, httptest.NewRequest(
		http.MethodPost, path+"/conversations/default/terminal", nil,
	))
	if start.Code != http.StatusAccepted {
		t.Fatal(start.Code)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var finished, startFinished chan struct{}
	t.Cleanup(func() {
		close(release)
		if finished != nil {
			<-finished
		}
		if startFinished != nil {
			<-startFinished
		}
	})
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("startup did not begin", ctx.Err())
	}
	deletion := httptest.NewRecorder()
	finished = make(chan struct{})
	go func() {
		defer close(finished)
		s.http.Handler.ServeHTTP(deletion, httptest.NewRequest(http.MethodDelete, path+"?discard=true", nil))
	}()
	select {
	case <-canceled:
	case <-ctx.Done():
		t.Fatal("deletion did not cancel startup", ctx.Err())
	}
	current, err := projects.Get(ctx, p.ID)
	if err != nil || current != p {
		t.Fatalf("project changed before startup cleanup finished: %+v %v", current, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "projects", p.ID, "repo", ".git")); err != nil {
		t.Fatal("removed files before startup cleanup finished", err)
	}
	rejected := httptest.NewRecorder()
	startFinished = make(chan struct{})
	go func() {
		defer close(startFinished)
		s.http.Handler.ServeHTTP(rejected, httptest.NewRequest(
			http.MethodPost, path+"/conversations/default/terminal", nil,
		))
	}()

	// A blocked shutdown must not prevent changes to another project.
	body, err := json.Marshal(map[string]any{"name": "Renamed", "version": other.Version})
	if err != nil {
		t.Fatal(err)
	}
	renamed := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(renamed, httptest.NewRequest(
		http.MethodPatch, "/api/projects/"+other.ID, strings.NewReader(string(body)),
	))
	if renamed.Code != http.StatusOK {
		t.Fatalf("rename other project: %d %s", renamed.Code, renamed.Body.String())
	}
	release <- struct{}{}
	<-finished
	<-startFinished
	if rejected.Code != http.StatusNotFound {
		t.Fatalf("start after deletion: %d", rejected.Code)
	}
	if deletion.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", deletion.Code, deletion.Body.String())
	}
}
