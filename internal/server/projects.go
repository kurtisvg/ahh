package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kurtisvg/ahh/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func projectError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusServiceUnavailable, "storage_unavailable", "Project storage is unavailable"
	if errors.Is(err, store.ErrNotFound) {
		status, code, message = http.StatusNotFound, "not_found", "Project not found"
	} else {
		slog.Error("project operation failed", "error", err)
	}
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetProject(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
