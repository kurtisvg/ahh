package server

import "net/http"

// exitResult is the harness result reported by wrap before it exits.
type exitResult struct {
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   string `json:"signal,omitempty"`
}

type processStatus struct {
	State      string `json:"state"`
	Invocation string `json:"invocation,omitempty"`
	Error      string `json:"error,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	Signal     string `json:"signal,omitempty"`
}

// currentWrapper snapshots the transitional terminal's concrete process.
func (s *Server) currentWrapper() *wrapperProcess {
	status := s.executions.Status("")
	child, _ := s.executions.Process("", status.Invocation)
	return child
}

func (s *Server) serveStart(w http.ResponseWriter, r *http.Request) {
	err := s.executions.startAndWait(r.Context(), "", s.startupDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	s.serveStatus(w, r)
}

func (s *Server) serveStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.executions.Status(""))
}
