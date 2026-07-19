package agents

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const ClaudeCodeHarness = "claude-code"

var (
	ErrNotFound           = errors.New("agent not found")
	ErrNameConflict       = errors.New("agent name already exists")
	ErrUnsupportedHarness = errors.New("unsupported agent harness")
)

// Config is the persisted, user-editable definition of an Agent.
type Config struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Harness string `json:"harness"`
}

// Manager owns persisted Agent definitions.
type Manager struct {
	mu     sync.Mutex
	agents map[string]Config
	store  *fileStore
}

// NewManager loads Agents beneath stateDir and creates the default Agent when
// the registry is empty.
func NewManager(stateDir string) (*Manager, error) {
	if strings.TrimSpace(stateDir) == "" {
		return nil, fmt.Errorf("agent state directory is required")
	}

	store := newFileStore(stateDir)
	stored, err := store.Load()
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		agents: make(map[string]Config, len(stored)),
		store:  store,
	}
	for _, agent := range stored {
		if err := validateConfig(agent); err != nil {
			return nil, fmt.Errorf("load agent %q: %w", agent.ID, err)
		}
		if manager.nameExists(agent.Name, "") {
			return nil, fmt.Errorf("load agent %q: %w", agent.ID, ErrNameConflict)
		}
		manager.agents[agent.ID] = agent
	}

	if len(manager.agents) == 0 {
		defaultAgent := Config{
			ID:      ClaudeCodeHarness,
			Name:    "Claude Code",
			Harness: ClaudeCodeHarness,
		}
		if err := manager.store.Save(defaultAgent); err != nil {
			return nil, fmt.Errorf("create default agent: %w", err)
		}
		manager.agents[defaultAgent.ID] = defaultAgent
	}

	return manager, nil
}

// Create persists a new Agent. Its generated ID remains stable across renames.
func (m *Manager) Create(name, harness string) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = strings.TrimSpace(name)
	harness = strings.TrimSpace(harness)
	if name == "" {
		return Config{}, fmt.Errorf("agent name is required")
	}
	if harness != ClaudeCodeHarness {
		return Config{}, fmt.Errorf("%w: %q", ErrUnsupportedHarness, harness)
	}
	if m.nameExists(name, "") {
		return Config{}, ErrNameConflict
	}

	idBase := normalizeID(name)
	id := idBase
	for suffix := 2; ; suffix++ {
		if _, exists := m.agents[id]; !exists {
			break
		}
		id = fmt.Sprintf("%s-%d", idBase, suffix)
	}

	agent := Config{ID: id, Name: name, Harness: harness}
	if err := m.store.Save(agent); err != nil {
		return Config{}, err
	}
	m.agents[id] = agent

	return agent, nil
}

// List returns Agents sorted case-insensitively by display name, then by ID.
func (m *Manager) List() []Config {
	m.mu.Lock()
	defer m.mu.Unlock()

	agents := make([]Config, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool {
		left := strings.ToLower(agents[i].Name)
		right := strings.ToLower(agents[j].Name)
		if left == right {
			return agents[i].ID < agents[j].ID
		}
		return left < right
	})

	return agents
}

// Get returns an Agent by its immutable ID.
func (m *Manager) Get(id string) (Config, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[id]
	return agent, ok
}

// Rename updates an Agent's display name without changing its ID or harness.
func (m *Manager) Rename(id, name string) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[id]
	if !ok {
		return Config{}, ErrNotFound
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Config{}, fmt.Errorf("agent name is required")
	}
	if m.nameExists(name, id) {
		return Config{}, ErrNameConflict
	}

	agent.Name = name
	if err := m.store.Save(agent); err != nil {
		return Config{}, err
	}
	m.agents[id] = agent

	return agent, nil
}

// ConfigDir returns the managed harness configuration directory for an Agent.
func (m *Manager) ConfigDir(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.agents[id]; !ok {
		return "", ErrNotFound
	}
	return m.store.ConfigDir(id)
}

func (m *Manager) nameExists(name, exceptID string) bool {
	for id, agent := range m.agents {
		if id != exceptID && strings.EqualFold(agent.Name, name) {
			return true
		}
	}
	return false
}

func validateConfig(agent Config) error {
	if agent.ID == "" || normalizeID(agent.ID) != agent.ID {
		return fmt.Errorf("invalid agent id %q", agent.ID)
	}
	if strings.TrimSpace(agent.Name) == "" {
		return fmt.Errorf("agent name is required")
	}
	if agent.Harness != ClaudeCodeHarness {
		return fmt.Errorf("%w: %q", ErrUnsupportedHarness, agent.Harness)
	}
	return nil
}

func normalizeID(name string) string {
	var id strings.Builder
	separatorPending := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if separatorPending && id.Len() > 0 {
				id.WriteByte('-')
			}
			id.WriteRune(r)
			separatorPending = false
		case unicode.IsSpace(r), unicode.IsPunct(r), unicode.IsSymbol(r):
			separatorPending = true
		default:
			separatorPending = true
		}
	}
	if id.Len() == 0 {
		return "agent"
	}
	return id.String()
}
