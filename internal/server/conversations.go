package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

type conversation struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Terminal processStatus `json:"terminal"`
}

func projectExecutionKey(p project.Project) string {
	// A recreated project must not inherit the previous project's terminal.
	return p.ID + ":" + p.CreatorToken
}

func (s *Server) terminalStatus(p project.Project) processStatus {
	status := s.executions.Status(projectExecutionKey(p))
	if status.State == "exited" {
		status.State = "stopped"
	}
	if status.State == "failed" {
		status.State = "disconnected"
		status.Error = "Harness exit is unconfirmed. Inspect the server before restarting."
	}
	return status
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	p, err := s.projects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, []conversation{{
		ID:       "default",
		Title:    "Conversation",
		Terminal: s.terminalStatus(p),
	}})
}

func (s *Server) conversationStatus(w http.ResponseWriter, r *http.Request) {
	p, err := s.projects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	status := s.terminalStatus(p)
	allowed := p.State == project.StateReady && *status.StartAllowed
	status.StartAllowed = &allowed
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) startConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lock := s.projectLock(id)
	lock.Lock()
	defer lock.Unlock()

	p, err := s.projects.Get(r.Context(), id)
	if err != nil {
		projectError(w, err)
		return
	}
	if p.State != project.StateReady {
		projectError(w, store.ErrConflict)
		return
	}
	directory, err := s.projectDirectory(p.ID)
	if err != nil {
		slog.Error("open project directory", "project", p.ID, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, apiError{
			Code:    "project_unavailable",
			Message: "Project directory is unavailable",
		})
		return
	}
	err = s.executions.Start(r.Context(), projectExecutionKey(p), directory)
	if err != nil {
		projectError(w, store.ErrConflict)
		return
	}
	writeJSON(w, http.StatusAccepted, s.terminalStatus(p))
}

func (s *Server) serveProjectTerminal(
	w http.ResponseWriter,
	r *http.Request,
	protection *http.CrossOriginProtection,
) {
	p, err := s.projects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	if p.State != project.StateReady {
		projectError(w, store.ErrConflict)
		return
	}
	child, err := s.executions.Process(projectExecutionKey(p), r.PathValue("invocation"))
	if err != nil {
		projectError(w, store.ErrConflict)
		return
	}
	select {
	case <-child.done:
		http.Error(w, "invocation has ended", http.StatusGone)
		return
	default:
	}
	proxyPTY(
		w,
		r,
		child.url,
		protection,
	)
}

// Check files when starting a conversation; ordinary project reads need no files.
func (s *Server) projectDirectory(id string) (string, error) {
	if err := project.ValidateID(id); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(s.Workspace)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	dir := filepath.Join("projects", id)
	repo := filepath.Join(dir, "repo")
	for _, path := range []string{"projects", dir, repo, filepath.Join(repo, ".git")} {
		info, err := root.Lstat(path)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%s is not a directory", path)
		}
	}
	return filepath.Join(s.Workspace, repo), nil
}
