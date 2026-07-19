package agents

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/harness"
)

const DefaultAgentName = "default"

var (
	ErrNotFound           = errors.New("agent not found")
	ErrNameConflict       = errors.New("agent name already exists")
	ErrImmutable          = errors.New("agent immutable fields cannot be changed")
	ErrUnsupportedHarness = errors.New("unsupported agent harness")
)

// Config is the persisted definition of an Agent.
type Config struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Harness harness.Type `json:"harness"`
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
			ID:      uuid.NewString(),
			Name:    DefaultAgentName,
			Harness: harness.ClaudeCode,
		}
		if err := manager.store.Save(defaultAgent); err != nil {
			return nil, fmt.Errorf("create default agent: %w", err)
		}
		manager.agents[defaultAgent.ID] = defaultAgent
	}

	return manager, nil
}

// Create persists a new Agent. Its generated ID remains stable across updates.
func (m *Manager) Create(name string, harnessType harness.Type) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = strings.TrimSpace(name)
	harnessType = harness.Type(strings.TrimSpace(string(harnessType)))
	if name == "" {
		return Config{}, fmt.Errorf("agent name is required")
	}
	if harnessType != harness.ClaudeCode {
		return Config{}, fmt.Errorf("%w: %q", ErrUnsupportedHarness, harnessType)
	}
	if m.nameExists(name, "") {
		return Config{}, ErrNameConflict
	}

	id := uuid.NewString()
	for {
		if _, exists := m.agents[id]; !exists {
			break
		}
		id = uuid.NewString()
	}

	agent := Config{ID: id, Name: name, Harness: harnessType}
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

// Update persists a complete Agent configuration after verifying its immutable fields.
func (m *Manager) Update(id string, next Config) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, ok := m.agents[id]
	if !ok {
		return Config{}, ErrNotFound
	}
	if next.ID != current.ID || next.Harness != current.Harness {
		return Config{}, ErrImmutable
	}
	next.Name = strings.TrimSpace(next.Name)
	if err := validateConfig(next); err != nil {
		return Config{}, err
	}
	if m.nameExists(next.Name, id) {
		return Config{}, ErrNameConflict
	}

	if err := m.store.Save(next); err != nil {
		return Config{}, err
	}
	m.agents[id] = next

	return next, nil
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
	id, err := uuid.Parse(agent.ID)
	if err != nil || id.Version() != 4 || id.String() != agent.ID {
		return fmt.Errorf("invalid agent id %q", agent.ID)
	}
	if strings.TrimSpace(agent.Name) == "" {
		return fmt.Errorf("agent name is required")
	}
	if agent.Harness != harness.ClaudeCode {
		return fmt.Errorf("%w: %q", ErrUnsupportedHarness, agent.Harness)
	}
	return nil
}
