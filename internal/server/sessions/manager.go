package sessions

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

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
	Resumable    bool      `json:"resumable,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// WrapperStart identifies the harness session a wrapper should create or resume.
type WrapperStart struct {
	SessionID string
	Resume    bool
}

// Session manages the mutable runtime state for one terminal session.
type Session struct {
	mu           sync.Mutex
	lifecycleMu  sync.Mutex
	metadata     Metadata
	wrapper      wrapper.Wrapper
	startWrapper func(WrapperStart) (wrapper.Wrapper, error)
	now          func() time.Time
	store        metadataStore
	deleted      bool
}

// Manager owns the session registry. Each Session owns its own mutable state.
type Manager struct {
	mu           sync.Mutex
	sessions     map[string]*Session
	closed       bool
	startWrapper func(WrapperStart) (wrapper.Wrapper, error)
	cancel       context.CancelFunc
	now          func() time.Time
	store        metadataStore
}

type options struct {
	startWrapper func(context.Context, WrapperStart) (wrapper.Wrapper, error)
	now          func() time.Time
	store        metadataStore
}

// Option configures a Manager.
type Option func(*options) error

// NewManager creates a session manager whose wrappers share the lifetime of ctx.
func NewManager(ctx context.Context, opts ...Option) (*Manager, error) {
	cfg := options{
		startWrapper: startWrapperSession,
		now:          func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	lifetimeCtx, cancel := context.WithCancel(ctx)

	manager := &Manager{
		sessions: map[string]*Session{},
		startWrapper: func(start WrapperStart) (wrapper.Wrapper, error) {
			if err := lifetimeCtx.Err(); err != nil {
				return nil, err
			}
			return cfg.startWrapper(lifetimeCtx, start)
		},
		cancel: cancel,
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
		manager.sessions[metadata.ID] = restoreSession(metadata, cfg.now, cfg.store, manager.startWrapper)
	}

	return manager, nil
}

// WithStateDir persists session metadata beneath stateDir.
func WithStateDir(stateDir string) Option {
	return func(opts *options) error {
		if strings.TrimSpace(stateDir) == "" {
			return fmt.Errorf("session state directory is required")
		}

		opts.store = newFileMetadataStore(stateDir)
		return nil
	}
}

// WithStartWrapper replaces the wrapper startup hook used when creating sessions.
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
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("session manager is shut down")
	}

	// Wrapper lifetime belongs to the manager, not the request creating it.
	w, err := m.startWrapper(WrapperStart{})
	if err != nil {
		return nil, err
	}
	id := w.SessionID()
	if id == "" {
		startErr := fmt.Errorf("started wrapper returned an empty session id")
		if shutdownErr := w.Shutdown(ctx); shutdownErr != nil {
			return nil, errors.Join(startErr, shutdownErr)
		}
		return nil, startErr
	}

	session := newSession(id, name, m.now, m.store, m.startWrapper)
	session.setWrapper(w)

	m.mu.Lock()
	_, exists := m.sessions[id]
	closed = m.closed
	if !exists && !closed {
		m.sessions[id] = session
	}
	m.mu.Unlock()

	var registrationErr error
	switch {
	case closed:
		registrationErr = fmt.Errorf("session manager is shut down")
	case exists:
		registrationErr = fmt.Errorf("session id %q already exists", id)
	}
	if registrationErr != nil {
		if shutdownErr := session.Shutdown(ctx); shutdownErr != nil {
			return nil, errors.Join(registrationErr, shutdownErr)
		}

		return nil, registrationErr
	}
	if m.store != nil {
		if err := m.store.Save(session.Metadata()); err != nil {
			m.remove(id, session)
			if shutdownErr := session.Shutdown(ctx); shutdownErr != nil {
				return nil, errors.Join(err, shutdownErr)
			}

			return nil, err
		}
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

	if err := session.delete(ctx); err != nil {
		return true, err
	}

	m.remove(id, session)
	return true, nil
}

// Shutdown stops every live session wrapper managed by m.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.Unlock()

	m.cancel()

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

func (m *Manager) remove(id string, session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if current := m.sessions[id]; current == session {
		delete(m.sessions, id)
	}
}

func newSession(
	id string,
	name string,
	now func() time.Time,
	store metadataStore,
	startWrapper func(WrapperStart) (wrapper.Wrapper, error),
) *Session {
	createdAt := now()
	return &Session{
		metadata: Metadata{
			ID:           id,
			Name:         name,
			Status:       StatusStarting,
			CreatedAt:    createdAt,
			LastActiveAt: createdAt,
		},
		startWrapper: startWrapper,
		now:          now,
		store:        store,
	}
}

func restoreSession(
	metadata Metadata,
	now func() time.Time,
	store metadataStore,
	startWrapper func(WrapperStart) (wrapper.Wrapper, error),
) *Session {
	metadata.Status = StatusExited
	return &Session{
		metadata:     metadata,
		startWrapper: startWrapper,
		now:          now,
		store:        store,
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
func (s *Session) Touch() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.deleted {
		return fmt.Errorf("session is deleted")
	}

	s.mu.Lock()
	s.metadata.LastActiveAt = s.now()
	metadata := s.metadata
	store := s.store
	s.mu.Unlock()

	if store == nil {
		return nil
	}
	if err := store.Save(metadata); err != nil {
		return fmt.Errorf("persist session activity: %w", err)
	}

	return nil
}

// MarkResumable records that Claude has received input for this session.
func (s *Session) MarkResumable() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.deleted {
		return fmt.Errorf("session is deleted")
	}

	s.mu.Lock()
	if s.metadata.Resumable {
		s.mu.Unlock()
		return nil
	}
	metadata := s.metadata
	metadata.Resumable = true
	store := s.store
	if store == nil {
		s.metadata.Resumable = true
	}
	s.mu.Unlock()

	if store == nil {
		return nil
	}
	if err := store.Save(metadata); err != nil {
		return fmt.Errorf("persist resumable session: %w", err)
	}
	s.mu.Lock()
	s.metadata.Resumable = true
	s.mu.Unlock()

	return nil
}

// Wrapper returns the current wrapper and lifecycle status for this session.
func (s *Session) Wrapper() (wrapper.Wrapper, Status) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.wrapper, s.metadata.Status
}

// Start returns the live wrapper, starting it when this session is not running.
func (s *Session) Start(ctx context.Context) (wrapper.Wrapper, error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if s.deleted {
		return nil, fmt.Errorf("session is deleted")
	}

	s.mu.Lock()
	if s.wrapper != nil {
		w := s.wrapper
		s.mu.Unlock()
		return w, nil
	}
	s.metadata.Status = StatusStarting
	s.mu.Unlock()

	metadata := s.Metadata()
	w, err := s.startWrapper(WrapperStart{
		SessionID: metadata.ID,
		Resume:    metadata.Resumable,
	})
	if err != nil {
		s.mu.Lock()
		s.metadata.Status = StatusExited
		s.mu.Unlock()
		return nil, fmt.Errorf("start session wrapper: %w", err)
	}
	if w.SessionID() != metadata.ID {
		startErr := fmt.Errorf(
			"started wrapper session id = %q, want %q",
			w.SessionID(),
			metadata.ID,
		)
		if shutdownErr := w.Shutdown(ctx); shutdownErr != nil {
			return nil, errors.Join(startErr, shutdownErr)
		}
		return nil, startErr
	}

	s.mu.Lock()
	s.wrapper = w
	s.metadata.Status = StatusRunning
	metadata = s.metadata
	store := s.store
	s.mu.Unlock()

	if store != nil {
		if err := store.Save(metadata); err != nil {
			persistErr := fmt.Errorf("persist started session: %w", err)
			shutdownErr := w.Shutdown(ctx)
			s.mu.Lock()
			if s.wrapper == w {
				s.wrapper = nil
				s.metadata.Status = StatusExited
			}
			s.mu.Unlock()
			if shutdownErr != nil {
				return nil, errors.Join(persistErr, shutdownErr)
			}
			return nil, persistErr
		}
	}

	go s.watchWrapper(w)
	return w, nil
}

// Shutdown stops this session's live wrapper, if one exists.
func (s *Session) Shutdown(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	return s.shutdown(ctx)
}

func (s *Session) shutdown(ctx context.Context) error {
	w, _ := s.Wrapper()
	if w == nil {
		return nil
	}
	if err := w.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown session wrapper: %w", err)
	}

	return nil
}

func (s *Session) delete(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

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
func startWrapperSession(ctx context.Context, start WrapperStart) (wrapper.Wrapper, error) {
	var opts []wrapper.Option
	switch {
	case start.Resume:
		opts = append(opts, wrapper.WithResume(start.SessionID))
	case start.SessionID != "":
		opts = append(opts, wrapper.WithSessionID(start.SessionID))
	}
	w, err := wrapper.Start(ctx, defaultWrapperHarness, defaultWrapperAddr, opts...)
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
