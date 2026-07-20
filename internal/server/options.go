package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kurtisvg/ahh/internal/server/agents"
	"github.com/kurtisvg/ahh/internal/server/conversations"
	"github.com/kurtisvg/ahh/internal/server/projects"
	"github.com/kurtisvg/ahh/internal/server/settings"
)

type options struct {
	agents        *agents.Manager
	conversations *conversations.Manager
	projects      *projects.Manager
	settings      *settings.Manager
	stateDir      string
}

// WithProjectManager sets the Project manager used by the server.
func WithProjectManager(manager *projects.Manager) Option {
	return func(opts *options) error {
		if manager == nil {
			return fmt.Errorf("project manager is required")
		}

		opts.projects = manager
		return nil
	}
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

// WithAgentManager sets the Agent manager used by the server.
func WithAgentManager(manager *agents.Manager) Option {
	return func(opts *options) error {
		if manager == nil {
			return fmt.Errorf("agent manager is required")
		}

		opts.agents = manager
		return nil
	}
}

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
