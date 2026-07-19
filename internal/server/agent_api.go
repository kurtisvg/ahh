package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/kurtisvg/ahh/internal/harness"
	"github.com/kurtisvg/ahh/internal/server/agents"
)

type createAgentRequest struct {
	Name    string `json:"name"`
	Harness string `json:"harness"`
}

type updateAgentRequest struct {
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
	harnessName := strings.TrimSpace(req.Harness)
	if harnessName == "" {
		writeAPIError(w, http.StatusBadRequest, "agent harness is required")
		return
	}
	harnessType, err := harness.ParseType(harnessName)
	if err != nil {
		writeAgentError(w, err)
		return
	}

	agent, err := s.agents.Create(name, harnessType)
	if err != nil {
		writeAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, agent)
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	var req updateAgentRequest
	if !decodeAgentRequest(w, r, &req) {
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAPIError(w, http.StatusBadRequest, "agent name is required")
		return
	}

	id := r.PathValue("id")
	current, ok := s.agents.Get(id)
	if !ok {
		writeAgentError(w, agents.ErrNotFound)
		return
	}
	current.Name = name
	agent, err := s.agents.Update(id, current)
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
	case errors.Is(err, agents.ErrUnsupportedHarness), errors.Is(err, harness.ErrUnsupportedType):
		writeAPIError(w, http.StatusBadRequest, "unsupported agent harness")
	default:
		writeAPIError(w, http.StatusInternalServerError, "agent operation failed")
	}
}
