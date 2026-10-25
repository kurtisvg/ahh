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

// currentWrapper returns a stable process reference; replacement never retargets a proxy.
func (s *Server) currentWrapper() *wrapperProcess {
	s.wrapperMu.Lock()
	defer s.wrapperMu.Unlock()
	return s.wrapper
}

func (s *Server) serveStart(w http.ResponseWriter, r *http.Request) {
	s.wrapperMu.Lock()
	child := s.wrapper
	if child != nil {
		select {
		case <-child.done:
			child = nil
		default:
		}
	}
	var err error
	if child == nil {
		child, err = s.launch()
		if err == nil {
			s.wrapper = child
		}
	}
	s.wrapperMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	s.serveStatus(w, r)
}

// serveStatus keeps the final result available after the wrapper has exited.
func (s *Server) serveStatus(w http.ResponseWriter, _ *http.Request) {
	status := processStatus{State: "stopped"}
	child := s.currentWrapper()
	if child != nil {
		status.State = "running"
		select {
		case <-child.done:
			status.State = "failed"
			if result := child.result; result != nil {
				status.State = "exited"
				status.ExitCode = result.ExitCode
				status.Signal = result.Signal
			}
		default:
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(status)
}
