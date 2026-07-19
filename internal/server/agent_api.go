package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/kurtisvg/ahh/internal/server/agents"
)

type createAgentRequest struct {
	Name    string `json:"name"`
	Harness string `json:"harness"`
}

type renameAgentRequest struct {
	Name string `json:"name"`
}

type agentsResponse struct {
	Agents []agents.Config `json:"agents"`
}

func (s *Server) listAgents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, agentsResponse{Agents: s.agents.List()})
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	var req createAgentRequest
	if !decodeAgentRequest(w, r, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAPIError(w, http.StatusBadRequest, "agent name is required")
		return
	}
	harness := strings.TrimSpace(req.Harness)
	if harness == "" {
		writeAPIError(w, http.StatusBadRequest, "agent harness is required")
		return
	}

	agent, err := s.agents.Create(name, harness)
	if err != nil {
		writeAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, agent)
}

func (s *Server) renameAgent(w http.ResponseWriter, r *http.Request) {
	var req renameAgentRequest
	if !decodeAgentRequest(w, r, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAPIError(w, http.StatusBadRequest, "agent name is required")
		return
	}

	agent, err := s.agents.Rename(r.PathValue("id"), name)
	if err != nil {
		writeAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func decodeAgentRequest(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid agent request")
		return false
	}
	return true
}

func writeAgentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, agents.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "agent not found")
	case errors.Is(err, agents.ErrNameConflict):
		writeAPIError(w, http.StatusConflict, "agent name already exists")
	case errors.Is(err, agents.ErrUnsupportedHarness):
		writeAPIError(w, http.StatusBadRequest, "unsupported agent harness")
	default:
		writeAPIError(w, http.StatusInternalServerError, "agent operation failed")
	}
}
