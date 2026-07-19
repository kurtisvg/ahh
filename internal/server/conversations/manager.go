package conversations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/harness"
	"github.com/kurtisvg/ahh/internal/server/agents"
	"github.com/kurtisvg/ahh/internal/wrapper"
)

const defaultWrapperAddr = "127.0.0.1:0"

var (
	ErrAgentRequired = errors.New("conversation agent is required")
	ErrAgentNotFound = errors.New("conversation agent not found")
)

type agentResolver interface {
	Get(id string) (agents.Config, bool)
	ConfigDir(id string) (string, error)
}

// Status is the lifecycle state reported for a user-created conversation.
type Status string

const (
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusExited   Status = "exited"
)

// Metadata is the user-facing metadata for one harness terminal conversation.
type Metadata struct {
	ID           string    `json:"id"`
	AgentID      string    `json:"agent_id"`
	Name         string    `json:"name"`
	Status       Status    `json:"status"`
	Resumable    bool      `json:"resumable,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// WrapperStart identifies the harness session a wrapper should create or resume
// for a conversation.
type WrapperStart struct {
	SessionID string
	Harness   harness.Type
	ConfigDir string
	Resume    bool
}

// Conversation manages the mutable runtime state for one terminal conversation.
type Conversation struct {
	startWrapper func(WrapperStart) (wrapper.Wrapper, error)
	agents       agentResolver
	now          func() time.Time
	store        metadataStore

	// operationMu serializes operations and protects the fields below it.
	operationMu sync.Mutex
	deleted     bool

	// stateMu protects the fields below it.
	stateMu  sync.Mutex
	metadata Metadata
	wrapper  wrapper.Wrapper
}

// Manager owns the conversation registry. Each Conversation owns its own mutable state.
type Manager struct {
	mu            sync.Mutex
	conversations map[string]*Conversation
	closed        bool
	startWrapper  func(WrapperStart) (wrapper.Wrapper, error)
	agents        agentResolver
	cancel        context.CancelFunc
	newID         func() (string, error)
	now           func() time.Time
	store         metadataStore
}

type options struct {
	startWrapper func(context.Context, WrapperStart) (wrapper.Wrapper, error)
	agents       agentResolver
	newID        func() (string, error)
	now          func() time.Time
	store        metadataStore
}

// Option configures a Manager.
type Option func(*options) error

// NewManager creates a conversation manager whose wrappers share the lifetime of ctx.
func NewManager(ctx context.Context, opts ...Option) (*Manager, error) {
	cfg := options{
		startWrapper: startWrapperConversation,
		newID:        newConversationID,
		now:          func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if cfg.agents == nil {
		return nil, fmt.Errorf("agent resolver is required")
	}
	lifetimeCtx, cancel := context.WithCancel(ctx)

	manager := &Manager{
		conversations: map[string]*Conversation{},
		startWrapper: func(start WrapperStart) (wrapper.Wrapper, error) {
			if err := lifetimeCtx.Err(); err != nil {
				return nil, err
			}
			return cfg.startWrapper(lifetimeCtx, start)
		},
		agents: cfg.agents,
		cancel: cancel,
		newID:  cfg.newID,
		now:    cfg.now,
		store:  cfg.store,
	}
	if cfg.store == nil {
		return manager, nil
	}

	stored, err := cfg.store.Load()
	if err != nil {
		cancel()
		return nil, err
	}
	for _, metadata := range stored {
		manager.conversations[metadata.ID] = restoreConversation(
			metadata,
			cfg.agents,
			cfg.now,
			cfg.store,
			manager.startWrapper,
		)
	}

	return manager, nil
}

// WithAgentResolver sets the Agent registry used for wrapper launch settings.
func WithAgentResolver(resolver agentResolver) Option {
	return func(opts *options) error {
		if resolver == nil {
			return fmt.Errorf("agent resolver is required")
		}
		opts.agents = resolver
		return nil
	}
}

// WithStateDir persists conversation metadata beneath stateDir.
func WithStateDir(stateDir string) Option {
	return func(opts *options) error {
		if strings.TrimSpace(stateDir) == "" {
			return fmt.Errorf("conversation state directory is required")
		}

		opts.store = newFileMetadataStore(stateDir)
		return nil
	}
}

// WithStartWrapper replaces the wrapper startup hook used when creating conversations.
func WithStartWrapper(
	start func(context.Context, WrapperStart) (wrapper.Wrapper, error),
) Option {
	return func(opts *options) error {
		if start == nil {
			return fmt.Errorf("wrapper start function is required")
		}

		opts.startWrapper = start
		return nil
	}
}

// WithIDGenerator replaces the conversation id generator.
func WithIDGenerator(newID func() (string, error)) Option {
	return func(opts *options) error {
		if newID == nil {
			return fmt.Errorf("conversation id generator is required")
		}

		opts.newID = newID
		return nil
	}
}

// WithClock replaces the clock used for conversation timestamps.
func WithClock(now func() time.Time) Option {
	return func(opts *options) error {
		if now == nil {
			return fmt.Errorf("conversation clock is required")
		}

		opts.now = now
		return nil
	}
}

// Create starts a wrapper for a named conversation and stores it in the registry.
func (m *Manager) Create(ctx context.Context, name, agentID string) (*Conversation, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("conversation name is required")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, ErrAgentRequired
	}
	agent, configDir, err := resolveAgent(m.agents, agentID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("conversation manager is shut down")
	}
	id, err := m.newID()
	if err != nil {
		return nil, fmt.Errorf("generate conversation id: %w", err)
	}

	// Wrapper lifetime belongs to the manager, not the request creating it.
	w, err := m.startWrapper(WrapperStart{
		SessionID: id,
		Harness:   agent.Harness,
		ConfigDir: configDir,
	})
	if err != nil {
		return nil, err
	}

	createdAt := m.now()
	conversation := &Conversation{
		metadata: Metadata{
			ID:           id,
			AgentID:      agent.ID,
			Name:         name,
			Status:       StatusStarting,
			CreatedAt:    createdAt,
			LastActiveAt: createdAt,
		},
		startWrapper: m.startWrapper,
		agents:       m.agents,
		now:          m.now,
		store:        m.store,
	}
	conversation.setWrapper(w)

	m.mu.Lock()
	_, exists := m.conversations[id]
	closed = m.closed
	if !exists && !closed {
		m.conversations[id] = conversation
	}
	m.mu.Unlock()

	var registrationErr error
	switch {
	case closed:
		registrationErr = fmt.Errorf("conversation manager is shut down")
	case exists:
		registrationErr = fmt.Errorf("conversation id %q already exists", id)
	}
	if registrationErr != nil {
		if shutdownErr := conversation.Shutdown(ctx); shutdownErr != nil {
			return nil, errors.Join(registrationErr, shutdownErr)
		}

		return nil, registrationErr
	}
	if m.store != nil {
		if err := m.store.Save(conversation.Metadata()); err != nil {
			m.remove(id, conversation)
			if shutdownErr := conversation.Shutdown(ctx); shutdownErr != nil {
				return nil, errors.Join(err, shutdownErr)
			}

			return nil, err
		}
	}
	go conversation.watchWrapper(w)

	return conversation, nil
}

// List returns conversation metadata with the most recently active conversation first.
func (m *Manager) List() []Metadata {
	conversations := m.all()
	metadata := make([]Metadata, 0, len(conversations))
	for _, conversation := range conversations {
		metadata = append(metadata, conversation.Metadata())
	}
	sortByActivity(metadata)

	return metadata
}

// Get returns the conversation with id.
func (m *Manager) Get(id string) (*Conversation, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conversation, ok := m.conversations[id]
	return conversation, ok
}

// Delete shuts down a conversation's live wrapper and removes it from the registry.
// If shutdown fails, the conversation remains listed so callers can retry or inspect
// its current state.
func (m *Manager) Delete(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	conversation, ok := m.conversations[id]
	if !ok {
		m.mu.Unlock()
		return false, nil
	}
	m.mu.Unlock()

	if err := conversation.delete(ctx); err != nil {
		return true, err
	}

	m.remove(id, conversation)
	return true, nil
}

// Shutdown stops every live conversation wrapper managed by m.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	conversations := make([]*Conversation, 0, len(m.conversations))
	for _, conversation := range m.conversations {
		conversations = append(conversations, conversation)
	}
	m.mu.Unlock()

	m.cancel()

	wrappers := make([]wrapper.Wrapper, 0, len(conversations))
	for _, conversation := range conversations {
		if conversationWrapper, _ := conversation.Wrapper(); conversationWrapper != nil {
			wrappers = append(wrappers, conversationWrapper)
		}
	}

	var errs []error
	for _, w := range wrappers {
		if err := w.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (m *Manager) all() []*Conversation {
	m.mu.Lock()
	defer m.mu.Unlock()

	conversations := make([]*Conversation, 0, len(m.conversations))
	for _, conversation := range m.conversations {
		conversations = append(conversations, conversation)
	}

	return conversations
}

func (m *Manager) remove(id string, conversation *Conversation) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if current := m.conversations[id]; current == conversation {
		delete(m.conversations, id)
	}
}

func restoreConversation(
	metadata Metadata,
	agentResolver agentResolver,
	now func() time.Time,
	store metadataStore,
	startWrapper func(WrapperStart) (wrapper.Wrapper, error),
) *Conversation {
	metadata.Status = StatusExited
	return &Conversation{
		metadata:     metadata,
		startWrapper: startWrapper,
		agents:       agentResolver,
		now:          now,
		store:        store,
	}
}

// ID returns the stable conversation id.
func (s *Conversation) ID() string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	return s.metadata.ID
}

// Metadata returns a copy of the conversation metadata suitable for JSON responses.
func (s *Conversation) Metadata() Metadata {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	return s.metadata
}

// Touch records user-visible activity for this conversation.
func (s *Conversation) Touch() error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	if s.deleted {
		return fmt.Errorf("conversation is deleted")
	}

	s.stateMu.Lock()
	s.metadata.LastActiveAt = s.now()
	metadata := s.metadata
	store := s.store
	s.stateMu.Unlock()

	if store == nil {
		return nil
	}
	if err := store.Save(metadata); err != nil {
		return fmt.Errorf("persist conversation activity: %w", err)
	}

	return nil
}

// MarkResumable records that Claude has received input for this conversation.
func (s *Conversation) MarkResumable() error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	if s.deleted {
		return fmt.Errorf("conversation is deleted")
	}

	s.stateMu.Lock()
	if s.metadata.Resumable {
		s.stateMu.Unlock()
		return nil
	}
	metadata := s.metadata
	metadata.Resumable = true
	store := s.store
	if store == nil {
		s.metadata.Resumable = true
	}
	s.stateMu.Unlock()

	if store == nil {
		return nil
	}
	if err := store.Save(metadata); err != nil {
		return fmt.Errorf("persist resumable conversation: %w", err)
	}
	s.stateMu.Lock()
	s.metadata.Resumable = true
	s.stateMu.Unlock()

	return nil
}

// Wrapper returns the current wrapper and lifecycle status for this conversation.
func (s *Conversation) Wrapper() (wrapper.Wrapper, Status) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	return s.wrapper, s.metadata.Status
}

// Start returns the live wrapper, starting it when this conversation is not running.
func (s *Conversation) Start(ctx context.Context) (wrapper.Wrapper, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	if s.deleted {
		return nil, fmt.Errorf("conversation is deleted")
	}

	s.stateMu.Lock()
	if s.wrapper != nil {
		w := s.wrapper
		s.stateMu.Unlock()
		return w, nil
	}
	s.metadata.Status = StatusStarting
	s.stateMu.Unlock()

	metadata := s.Metadata()
	agent, configDir, err := resolveAgent(s.agents, metadata.AgentID)
	if err != nil {
		s.stateMu.Lock()
		s.metadata.Status = StatusExited
		s.stateMu.Unlock()
		return nil, err
	}
	w, err := s.startWrapper(WrapperStart{
		SessionID: metadata.ID,
		Harness:   agent.Harness,
		ConfigDir: configDir,
		Resume:    metadata.Resumable,
	})
	if err != nil {
		s.stateMu.Lock()
		s.metadata.Status = StatusExited
		s.stateMu.Unlock()
		return nil, fmt.Errorf("start conversation wrapper: %w", err)
	}
	s.stateMu.Lock()
	s.wrapper = w
	s.metadata.Status = StatusRunning
	metadata = s.metadata
	store := s.store
	s.stateMu.Unlock()

	if store != nil {
		if err := store.Save(metadata); err != nil {
			persistErr := fmt.Errorf("persist started conversation: %w", err)
			shutdownErr := w.Shutdown(ctx)
			s.stateMu.Lock()
			if s.wrapper == w {
				s.wrapper = nil
				s.metadata.Status = StatusExited
			}
			s.stateMu.Unlock()
			if shutdownErr != nil {
				return nil, errors.Join(persistErr, shutdownErr)
			}
			return nil, persistErr
		}
	}

	go s.watchWrapper(w)
	return w, nil
}

// Shutdown stops this conversation's live wrapper, if one exists.
func (s *Conversation) Shutdown(ctx context.Context) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	return s.shutdown(ctx)
}

func (s *Conversation) shutdown(ctx context.Context) error {
	w, _ := s.Wrapper()
	if w == nil {
		return nil
	}
	if err := w.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown conversation wrapper: %w", err)
	}

	return nil
}

func (s *Conversation) delete(ctx context.Context) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()

	if s.deleted {
		return nil
	}
	if err := s.shutdown(ctx); err != nil {
		return err
	}
	if s.store != nil {
		if err := s.store.Delete(s.ID()); err != nil {
			return err
		}
	}
	s.deleted = true

	return nil
}

func (s *Conversation) setWrapper(w wrapper.Wrapper) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.wrapper = w
	s.metadata.Status = StatusRunning
}

func (s *Conversation) watchWrapper(w wrapper.Wrapper) {
	_ = w.Wait()

	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.wrapper != w {
		return
	}

	s.metadata.Status = StatusExited
	s.wrapper = nil
}

// startWrapperConversation starts the configured wrapper backing a conversation.
func startWrapperConversation(ctx context.Context, start WrapperStart) (wrapper.Wrapper, error) {
	opts := []wrapper.Option{wrapper.WithConfigDir(start.ConfigDir)}
	if start.Resume {
		opts = append(opts, wrapper.WithResume())
	}
	w, err := wrapper.Start(
		ctx,
		start.Harness,
		defaultWrapperAddr,
		start.SessionID,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("start wrapper server: %w", err)
	}

	return w, nil
}

func resolveAgent(resolver agentResolver, id string) (agents.Config, string, error) {
	agent, ok := resolver.Get(id)
	if !ok {
		return agents.Config{}, "", fmt.Errorf("%w: %q", ErrAgentNotFound, id)
	}
	configDir, err := resolver.ConfigDir(id)
	if err != nil {
		if errors.Is(err, agents.ErrNotFound) {
			return agents.Config{}, "", fmt.Errorf("%w: %q", ErrAgentNotFound, id)
		}
		return agents.Config{}, "", fmt.Errorf("resolve Agent config directory: %w", err)
	}
	return agent, configDir, nil
}

// sortByActivity orders conversations for the list API.
func sortByActivity(metadata []Metadata) {
	sort.Slice(metadata, func(i, j int) bool {
		left := metadata[i]
		right := metadata[j]
		if !left.LastActiveAt.Equal(right.LastActiveAt) {
			return left.LastActiveAt.After(right.LastActiveAt)
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}

		return left.ID < right.ID
	})
}

// newConversationID generates an opaque random UUID v4 for bookmarkable conversation URLs.
func newConversationID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}

	return id.String(), nil
}
