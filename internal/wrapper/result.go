package wrapper

import (
	"errors"
	"os/exec"
	"syscall"
)

// ExitResult describes the harness exit, including code or signal when known.
type ExitResult struct {
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   string `json:"signal,omitempty"`
}

// Result waits for wrapper cleanup and returns the harness's exit result.
func (s *Server) Result() ExitResult {
	<-s.done
	status := ExitResult{}
	if s.harnessErr == nil {
		code := 0
		status.ExitCode = &code
		return status
	}

	var exit *exec.ExitError
	if errors.As(s.harnessErr, &exit) {
		code := exit.ExitCode()
		status.ExitCode = &code
		if result, ok := exit.Sys().(syscall.WaitStatus); ok && result.Signaled() {
			status.Signal = result.Signal().String()
		}
	}
	return status
}
