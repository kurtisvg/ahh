package server

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
