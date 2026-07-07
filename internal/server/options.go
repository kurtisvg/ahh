package server

import "fmt"

type options struct {
	sessions *SessionManager
}

// Option configures the Ahh server.
type Option func(*options) error

// WithSessionManager sets the session manager used by the server.
func WithSessionManager(sessions *SessionManager) Option {
	return func(opts *options) error {
		if sessions == nil {
			return fmt.Errorf("session manager is required")
		}

		opts.sessions = sessions
		return nil
	}
}
