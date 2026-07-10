package sessions

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/wrapper"
)

const (
	defaultWrapperAddr    = "127.0.0.1:0"
	defaultWrapperHarness = wrapper.ClaudeCodeHarness
)

// Status is the lifecycle state reported for a user-created session.
type Status string

const (
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusExited   Status = "exited"
)

// Metadata is the user-facing metadata for one harness terminal session.
type Metadata struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       Status    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// Session manages the mutable runtime state for one terminal session.
type Session struct {
	mu       sync.Mutex
	metadata Metadata
	wrapper  wrapper.Wrapper
	now      func() time.Time
}

// Manager owns the session registry. Each Session owns its own mutable state.
type Manager struct {
	mu           sync.Mutex
	sessions     map[string]*Session
	startWrapper func(context.Context) (wrapper.Wrapper, error)
	newID        func() (string, error)
	now          func() time.Time
}

type options struct {
	startWrapper func(context.Context) (wrapper.Wrapper, error)
	newID        func() (string, error)
	now          func() time.Time
}

// Option configures a Manager.
type Option func(*options) error

// NewManager creates the production session manager used by the server.
func NewManager(opts ...Option) (*Manager, error) {
	cfg := options{
		startWrapper: startWrapperSession,
		newID:        newSessionID,
		now:          func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	return &Manager{
		sessions:     map[string]*Session{},
		startWrapper: cfg.startWrapper,
		newID:        cfg.newID,
		now:          cfg.now,
	}, nil
}

// WithStartWrapper replaces the wrapper startup hook used when creating sessions.
func WithStartWrapper(start func(context.Context) (wrapper.Wrapper, error)) Option {
	return func(opts *options) error {
		if start == nil {
			return fmt.Errorf("wrapper start function is required")
		}

		opts.startWrapper = start
		return nil
	}
}

// WithIDGenerator replaces the session id generator.
func WithIDGenerator(newID func() (string, error)) Option {
	return func(opts *options) error {
		if newID == nil {
			return fmt.Errorf("session id generator is required")
		}

		opts.newID = newID
		return nil
	}
}

// WithClock replaces the clock used for session timestamps.
func WithClock(now func() time.Time) Option {
	return func(opts *options) error {
		if now == nil {
			return fmt.Errorf("session clock is required")
		}

		opts.now = now
		return nil
	}
}

// Create starts a wrapper for a named session and stores it in the registry.
func (m *Manager) Create(ctx context.Context, name string) (*Session, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("session name is required")
	}

	id, err := m.newID()
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	session := newSession(id, name, m.now)

	w, err := m.startWrapper(ctx)
	if err != nil {
		return nil, err
	}

	session.setWrapper(w)
	if err := m.add(id, session); err != nil {
		if shutdownErr := session.Shutdown(ctx); shutdownErr != nil {
			return nil, errors.Join(err, shutdownErr)
		}

		return nil, err
	}
	go session.watchWrapper(w)

	return session, nil
}

// List returns session metadata with the most recently active session first.
func (m *Manager) List() []Metadata {
	sessions := m.all()
	metadata := make([]Metadata, 0, len(sessions))
	for _, session := range sessions {
		metadata = append(metadata, session.Metadata())
	}
	sortByActivity(metadata)

	return metadata
}

// Get returns the session with id.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	return session, ok
}

// Delete shuts down a session's live wrapper and removes it from the registry.
// If shutdown fails, the session remains listed so callers can retry or inspect
// its current state.
func (m *Manager) Delete(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return false, nil
	}
	m.mu.Unlock()

	if err := session.Shutdown(ctx); err != nil {
		return true, err
	}

	m.remove(id, session)
	return true, nil
}

// Shutdown stops every live session wrapper managed by m.
func (m *Manager) Shutdown(ctx context.Context) error {
	sessions := m.all()
	wrappers := make([]wrapper.Wrapper, 0, len(sessions))
	for _, session := range sessions {
		if sessionWrapper, _ := session.Wrapper(); sessionWrapper != nil {
			wrappers = append(wrappers, sessionWrapper)
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

func (m *Manager) all() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

func (m *Manager) add(id string, session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[id]; ok {
		return fmt.Errorf("session id %q already exists", id)
	}

	m.sessions[id] = session
	return nil
}

func (m *Manager) remove(id string, session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if current := m.sessions[id]; current == session {
		delete(m.sessions, id)
	}
}

func newSession(id string, name string, now func() time.Time) *Session {
	createdAt := now()
	return &Session{
		metadata: Metadata{
			ID:           id,
			Name:         name,
			Status:       StatusStarting,
			CreatedAt:    createdAt,
			LastActiveAt: createdAt,
		},
		now: now,
	}
}

// ID returns the stable session id.
func (s *Session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.metadata.ID
}

// Metadata returns a copy of the session metadata suitable for JSON responses.
func (s *Session) Metadata() Metadata {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.metadata
}

// Touch records user-visible activity for this session.
func (s *Session) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metadata.LastActiveAt = s.now()
}

// Wrapper returns the current wrapper and lifecycle status for this session.
func (s *Session) Wrapper() (wrapper.Wrapper, Status) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.wrapper, s.metadata.Status
}

// Shutdown stops this session's live wrapper, if one exists.
func (s *Session) Shutdown(ctx context.Context) error {
	w, _ := s.Wrapper()
	if w == nil {
		return nil
	}
	if err := w.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown session wrapper: %w", err)
	}

	return nil
}

func (s *Session) setWrapper(w wrapper.Wrapper) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.wrapper = w
	s.metadata.Status = StatusRunning
}

func (s *Session) watchWrapper(w wrapper.Wrapper) {
	_ = w.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wrapper != w {
		return
	}

	s.metadata.Status = StatusExited
	s.wrapper = nil
}

// startWrapperSession starts the default wrapper backing a new session.
func startWrapperSession(ctx context.Context) (wrapper.Wrapper, error) {
	w, err := wrapper.Start(ctx, defaultWrapperHarness, defaultWrapperAddr)
	if err != nil {
		return nil, fmt.Errorf("start wrapper server: %w", err)
	}

	return w, nil
}

// sortByActivity orders sessions for the list API.
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

// newSessionID generates an opaque random UUID v4 for bookmarkable session URLs.
func newSessionID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}

	return id.String(), nil
}
