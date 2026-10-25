package server

import "net/http"

// exitResult is the harness result reported by wrap before it exits.
type exitResult struct {
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   string `json:"signal,omitempty"`
}

type processStatus struct {
	State        string `json:"state"`
	Invocation   string `json:"invocation,omitempty"`
	Error        string `json:"error,omitempty"`
	ExitCode     *int   `json:"exit_code,omitempty"`
	Signal       string `json:"signal,omitempty"`
	StartAllowed *bool  `json:"start_allowed,omitempty"`
}

// currentWrapper snapshots the transitional terminal's concrete process.
func (s *Server) currentWrapper() *wrapperProcess {
	status := s.executions.Status("")
	child, _ := s.executions.Process("", status.Invocation)
	return child
}

func (s *Server) serveTerminal(
	w http.ResponseWriter,
	r *http.Request,
	protection *http.CrossOriginProtection,
) {
	child := s.currentWrapper()
	if child == nil {
		http.Error(w, "terminal has not started", http.StatusConflict)
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
