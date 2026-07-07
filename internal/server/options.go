package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kurtisvg/ahh/internal/server/sessions"
)

type options struct {
	sessions  *sessions.Manager
	configDir string
}

// WithConfigDir sets the directory used for persisted Ahh metadata.
func WithConfigDir(configDir string) Option {
	return func(opts *options) error {
		opts.configDir = configDir
		return nil
	}
}

func defaultConfigDir() string {
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
