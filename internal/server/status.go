package server

import (
	"encoding/json"
	"net/http"
)

// exitResult is the harness result reported by wrap before it exits.
type exitResult struct {
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   string `json:"signal,omitempty"`
}

type processStatus struct {
	State    string `json:"state"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   string `json:"signal,omitempty"`
}

// serveStatus keeps the final result available after the wrapper has exited.
func (s *Server) serveStatus(w http.ResponseWriter, _ *http.Request) {
	status := processStatus{State: "running"}
	select {
	case <-s.wrapper.done:
		status.State = "failed"
		if result := s.wrapper.result; result != nil {
			status.State = "exited"
			status.ExitCode = result.ExitCode
			status.Signal = result.Signal
		}
	default:
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(status)
}
