package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store/sqlite"
)

func setupProjectServer(t *testing.T) (*Server, project.Manager, string) {
	t.Helper()
	useTestWrapper(t)
	workspace := t.TempDir()
	storage, err := sqlite.Open(t.Context(), filepath.Join(workspace, "ahh.db"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := startWithStore(
		t, t.Context(), Config{Address: "127.0.0.1", Workspace: workspace},
		storage, os.Args[0], "-test.run=^TestChildWrapper$",
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.Shutdown(context.Background())
		_ = s.Wait()
	})
	return s, s.projects, workspace
}

func readyProject(t *testing.T, manager project.Manager, name string) project.Project {
	t.Helper()
	p, err := manager.Create(t.Context(), strings.ToLower(name), name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProjectTerminals(t *testing.T) {
	s, projects, workspace := setupProjectServer(t)
	missing := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(
		missing,
		httptest.NewRequest(http.MethodGet, "/api/projects/missing/conversations", nil),
	)
	if missing.Code != http.StatusNotFound {
		t.Fatal(missing.Code)
	}
	var children []*wrapperProcess
	for _, name := range []string{"Alpha", "Beta"} {
		p := readyProject(t, projects, name)
		endpoint := "/api/projects/" + p.ID + "/conversations/default/terminal"
		var calls sync.WaitGroup
		for range 3 {
			calls.Go(func() {
				response := httptest.NewRecorder()
				s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint, nil))
				if response.Code != http.StatusAccepted {
					t.Errorf("start: %d %s", response.Code, response.Body.String())
				}
			})
		}
		calls.Wait()
		status := waitForProjectTerminal(t, s, endpoint)
		id := status.Invocation
		child, err := s.executions.Process(projectExecutionKey(p), id)
		if err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
		response := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, endpoint+"/stale/pty", nil),
		)
		if response.Code != http.StatusConflict {
			t.Fatal(response.Code)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		url := fmt.Sprintf("ws://127.0.0.1:%d%s/%s/pty", s.Port, endpoint, id)
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		var replay any
		if err := wsjson.Read(ctx, conn, &replay); err != nil {
			t.Fatal(err)
		}
		if err := wsjson.Write(ctx, conn, map[string]string{
			"type": "input",
			"data": name + "\n",
		}); err != nil {
			t.Fatal(err)
		}
		var output strings.Builder
		for !strings.Contains(output.String(), "received: "+name) {
			kind, data, err := conn.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if kind == websocket.MessageBinary && len(data) >= 8 {
				output.Write(data[8:])
			}
		}
		if !strings.Contains(output.String(), filepath.Join(workspace, "projects", p.ID, "repo")) {
			t.Fatalf("wrong working directory: %s", output.String())
		}
		_ = conn.CloseNow()
		cancel()
	}
	if children[0] == children[1] {
		t.Fatal("Projects share a wrapper")
	}
	p, err := projects.Get(t.Context(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	invocation := s.terminalStatus(p).Invocation
	if err := s.executions.Stop(t.Context(), projectExecutionKey(p)); err != nil {
		t.Fatal(err)
	}
	ended := httptest.NewRecorder()
	endpoint := "/api/projects/alpha/conversations/default/terminal/" + invocation + "/pty"
	s.http.Handler.ServeHTTP(ended, httptest.NewRequest(http.MethodGet, endpoint, nil))
	if ended.Code != http.StatusGone {
		t.Fatalf("ended invocation: %d %s", ended.Code, ended.Body.String())
	}
	if err := s.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	for _, child := range children {
		select {
		case <-child.done:
		default:
			t.Fatal("child survived shutdown")
		}
	}
	response := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/projects/missing/conversations", nil),
	)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal(response.Code)
	}
}

func TestMissingRepositoryCannotStart(t *testing.T) {
	s, projects, workspace := setupProjectServer(t)
	p := readyProject(t, projects, "Missing")
	repo := filepath.Join(workspace, "projects", p.ID, "repo")
	if err := os.Rename(repo, repo+"-saved"); err != nil {
		t.Fatal(err)
	}
	read := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/projects/"+p.ID, nil))
	if read.Code != http.StatusOK {
		t.Fatalf("missing files hid project: %d", read.Code)
	}
	endpoint := "/api/projects/" + p.ID + "/conversations/default/terminal"
	response := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, endpoint, nil))
	if !strings.Contains(response.Body.String(), `"start_allowed":true`) {
		t.Fatal(response.Body.String())
	}
	response = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint, nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal(response.Code)
	}
	if status := s.terminalStatus(p); status.Invocation != "" {
		t.Fatal("started outside Project repository")
	}
	if _, err := os.Stat(repo); !os.IsNotExist(err) {
		t.Fatal("recreated missing repository", err)
	}
}

func waitForProjectTerminal(t *testing.T, s *Server, endpoint string) processStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		response := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, endpoint, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status: %d %s", response.Code, response.Body.String())
		}
		var status processStatus
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.State == "running" {
			return status
		}
		if status.State != "starting" {
			t.Fatalf("unexpected terminal status: %+v", status)
		}
		select {
		case <-ctx.Done():
			t.Fatal("terminal did not start", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestProjectTerminalRejectsDirectoryAliases(t *testing.T) {
	for _, name := range []string{"repo", ".git"} {
		t.Run(name, func(t *testing.T) {
			s, projects, workspace := setupProjectServer(t)
			p := readyProject(t, projects, "linked")
			path := filepath.Join(workspace, "projects", p.ID, "repo")
			if name == ".git" {
				path = filepath.Join(path, name)
			}
			if err := os.Rename(path, path+"-saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(path+"-saved", path); err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			endpoint := "/api/projects/" + p.ID + "/conversations/default/terminal"
			s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint, nil))
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("started through alias: %d", response.Code)
			}
		})
	}
}
