package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/kurtisvg/ahh/internal/project"
	"github.com/kurtisvg/ahh/internal/store"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeBody(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err == nil && decoder.Decode(new(any)) == io.EOF {
		return true
	}
	writeJSON(w, http.StatusBadRequest, apiError{
		Code:    "invalid_request",
		Message: "Invalid request body",
	})
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func projectError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	response := apiError{
		Code:    "storage_unavailable",
		Message: "Project storage is unavailable",
	}
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
		response = apiError{
			Code:    "not_found",
			Message: "Project not found",
		}
	} else {
		slog.Error("project operation failed", "error", err)
	}
	writeJSON(w, status, response)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.projects.List(r.Context())
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.projects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := project.ValidateID(body.ID); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "invalid_request",
			Message: err.Error(),
		})
		return
	}
	p, err := s.projects.Create(r.Context(), body.ID)
	if errors.Is(err, store.ErrConflict) || errors.Is(err, os.ErrExist) {
		writeJSON(w, http.StatusConflict, apiError{
			Code:    "conflict",
			Message: "Project ID is taken or its directory already exists",
		})
		return
	}
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}
