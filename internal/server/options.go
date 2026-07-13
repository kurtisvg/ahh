package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kurtisvg/ahh/internal/server/conversations"
)

type options struct {
	conversations *conversations.Manager
	stateDir      string
}

// WithStateDir sets the directory used for persistent Ahh state.
func WithStateDir(stateDir string) Option {
	return func(opts *options) error {
		if strings.TrimSpace(stateDir) == "" {
			return fmt.Errorf("state directory is required")
		}
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

// WithConversationManager sets the conversation manager used by the server.
func WithConversationManager(manager *conversations.Manager) Option {
	return func(opts *options) error {
		if manager == nil {
			return fmt.Errorf("conversation manager is required")
		}

		opts.conversations = manager
		return nil
	}
}
