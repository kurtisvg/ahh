package server

import (
	"errors"
	"net/http"

	"github.com/kurtisvg/ahh/internal/server/projects"
)

type projectsResponse struct {
	Projects []projects.Metadata `json:"projects"`
}

type branchesResponse struct {
	Branches []projects.Branch `json:"branches"`
}

type createProjectRequest struct {
	Name   string          `json:"name"`
	Source projects.Source `json:"source"`
}

type updateProjectRequest struct {
	DefaultBranch projects.Branch `json:"default_branch"`
}

func (s *Server) listProjects(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, projectsResponse{Projects: s.projects.List()})
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid project request")
		return
	}
	if req.Source.Type != projects.SourceGitHub {
		writeAPIError(w, http.StatusBadRequest, "project source type must be github")
		return
	}
	created, err := s.projects.Create(r.Context(), req.Name, req.Source.Repository)
	switch {
	case errors.Is(err, projects.ErrNameRequired):
		writeAPIError(w, http.StatusBadRequest, "project name is required")
	case errors.Is(err, projects.ErrNameInvalid):
		writeAPIError(w, http.StatusBadRequest, "project name must be a URL-safe identifier of 1-64 letters, numbers, dots, underscores, or hyphens")
	case errors.Is(err, projects.ErrNameExists):
		writeAPIError(w, http.StatusConflict, "project name already exists")
	case err != nil:
		writeAPIError(w, http.StatusBadRequest, "project repository must be owner/repository")
	default:
		writeJSON(w, http.StatusCreated, created)
	}
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	project, ok := s.projects.Get(r.PathValue("project_id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, project.Metadata())
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	var req updateProjectRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid project request")
		return
	}
	updated, err := s.projects.UpdateDefaultBranch(r.Context(), r.PathValue("project_id"), req.DefaultBranch)
	switch {
	case errors.Is(err, projects.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, projects.ErrBranchNotFound):
		writeAPIError(w, http.StatusBadRequest, "project default branch does not exist")
	case err != nil:
		writeAPIError(w, http.StatusBadRequest, "project default branch is invalid")
	default:
		writeJSON(w, http.StatusOK, updated)
	}
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.projects.Delete(r.PathValue("project_id"))
	if errors.Is(err, projects.ErrDeleting) {
		writeAPIError(w, http.StatusConflict, "project deletion is already in progress")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "delete project failed")
		return
	}
	if !deleted {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listProjectBranches(w http.ResponseWriter, r *http.Request) {
	branches, err := s.projects.Branches(r.Context(), r.PathValue("project_id"))
	if errors.Is(err, projects.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "project branches are unavailable")
		return
	}
	writeJSON(w, http.StatusOK, branchesResponse{Branches: branches})
}

func (s *Server) fetchProject(w http.ResponseWriter, r *http.Request) {
	updated, err := s.projects.Fetch(r.Context(), r.PathValue("project_id"))
	if errors.Is(err, projects.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "project fetch failed")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
