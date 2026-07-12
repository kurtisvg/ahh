package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kurtisvg/ahh/internal/server/sessions"
)

type options struct {
	sessions *sessions.Manager
	stateDir string
}

// WithStateDir sets the directory used for persistent Ahh state.
func WithStateDir(stateDir string) Option {
	return func(opts *options) error {
		opts.stateDir = stateDir
		return nil
	}
}

func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".ahh"
	}

	return filepath.Join(home, ".ahh")
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
