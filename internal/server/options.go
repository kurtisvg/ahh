package server

import (
	"fmt"

	"github.com/kurtisvg/ahh/internal/server/sessions"
)

type options struct {
	sessions *sessions.Manager
}

// Option configures the Ahh server.
type Option func(*options) error

// WithSessionManager sets the session manager used by the server.
func WithSessionManager(manager *sessions.Manager) Option {
	return func(opts *options) error {
		if manager == nil {
			return fmt.Errorf("session manager is required")
		}

		opts.sessions = manager
		return nil
	}
}
