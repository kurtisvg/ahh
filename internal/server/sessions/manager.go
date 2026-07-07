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

// Session is the user-facing metadata for one harness terminal session.
type Session struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       Status    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

type managedSession struct {
	Session
	wrapper wrapper.Wrapper
}

// Manager owns live session wrappers and their user-facing metadata.
type Manager struct {
	mu           sync.Mutex
	sessions     map[string]*managedSession
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

// New creates the production session manager used by the server.
func New(opts ...Option) (*Manager, error) {
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
		sessions:     map[string]*managedSession{},
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

// Create starts a wrapper for a named session and stores its metadata.
func (m *Manager) Create(ctx context.Context, name string) (Session, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Session{}, fmt.Errorf("session name is required")
	}

	id, err := m.newID()
	if err != nil {
		return Session{}, fmt.Errorf("generate session id: %w", err)
	}

	now := m.now()
	session := &managedSession{
		Session: Session{
			ID:           id,
			Name:         name,
			Status:       StatusStarting,
			CreatedAt:    now,
			LastActiveAt: now,
		},
	}

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	w, err := m.startWrapper(ctx)
	if err != nil {
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()

		return Session{}, err
	}

	m.mu.Lock()
	session.wrapper = w
	session.Status = StatusRunning
	created := session.Session
	m.mu.Unlock()

	go m.watchWrapper(id, w)

	return created, nil
}

// List returns sessions with the most recently active session first.
func (m *Manager) List() []Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions := make([]Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session.Session)
	}
	sortByActivity(sessions)

	return sessions
}

// Touch records user activity for a session.
func (m *Manager) Touch(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return false
	}

	session.LastActiveAt = m.now()
	return true
}

// LookupWrapper returns the live wrapper for a session, if one exists.
func (m *Manager) LookupWrapper(id string) (wrapper.Wrapper, Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, "", false
	}

	return session.wrapper, session.Status, true
}

// Shutdown stops every live session wrapper managed by m.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	wrappers := make([]wrapper.Wrapper, 0, len(m.sessions))
	for _, session := range m.sessions {
		if session.wrapper != nil {
			wrappers = append(wrappers, session.wrapper)
		}
	}
	m.mu.Unlock()

	var errs []error
	for _, w := range wrappers {
		if err := w.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// watchWrapper updates session state after its wrapper exits.
func (m *Manager) watchWrapper(id string, w wrapper.Wrapper) {
	_ = w.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok || session.wrapper != w {
		return
	}

	session.Status = StatusExited
	session.wrapper = nil
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
func sortByActivity(sessions []Session) {
	sort.Slice(sessions, func(i, j int) bool {
		left := sessions[i]
		right := sessions[j]
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
