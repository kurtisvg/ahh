package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kurtisvg/ahh/internal/wrapper"
)

// SessionStatus is the lifecycle state reported for a user-created session.
type SessionStatus string

const (
	SessionStatusStarting SessionStatus = "starting"
	SessionStatusRunning  SessionStatus = "running"
	SessionStatusExited   SessionStatus = "exited"
)

// Session is the user-facing metadata for one harness terminal session.
type Session struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Status       SessionStatus `json:"status"`
	CreatedAt    time.Time     `json:"created_at"`
	LastActiveAt time.Time     `json:"last_active_at"`
}

type wrapperStarter func(context.Context) (wrapper.Wrapper, error)

type managedSession struct {
	Session
	wrapper wrapper.Wrapper
}

// SessionManager owns live session wrappers and their user-facing metadata.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*managedSession
	start    wrapperStarter
	newID    func() (string, error)
	now      func() time.Time
}

func newSessionManager(start wrapperStarter) *SessionManager {
	return &SessionManager{
		sessions: map[string]*managedSession{},
		start:    start,
		newID:    newUUID,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func newWrapperSessionManager() *SessionManager {
	return newSessionManager(func(ctx context.Context) (wrapper.Wrapper, error) {
		w, err := wrapper.Start(ctx, defaultWrapperHarness, defaultWrapperAddr)
		if err != nil {
			return nil, fmt.Errorf("start wrapper server: %w", err)
		}

		return w, nil
	})
}

func (m *SessionManager) Create(ctx context.Context, name string) (Session, error) {
	id, err := m.newID()
	if err != nil {
		return Session{}, fmt.Errorf("generate session id: %w", err)
	}

	now := m.now()
	session := &managedSession{
		Session: Session{
			ID:           id,
			Name:         name,
			Status:       SessionStatusStarting,
			CreatedAt:    now,
			LastActiveAt: now,
		},
	}

	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	w, err := m.start(ctx)
	if err != nil {
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()

		return Session{}, err
	}

	m.mu.Lock()
	session.wrapper = w
	session.Status = SessionStatusRunning
	created := session.Session
	m.mu.Unlock()

	go m.watch(id, w)

	return created, nil
}

func (m *SessionManager) List() []Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions := make([]Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session.Session)
	}
	sortSessions(sessions)

	return sessions
}

func (m *SessionManager) Touch(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return false
	}

	session.LastActiveAt = m.now()
	return true
}

func (m *SessionManager) Wrapper(id string) (wrapper.Wrapper, SessionStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, "", false
	}

	return session.wrapper, session.Status, true
}

func (m *SessionManager) Shutdown(ctx context.Context) error {
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

func (m *SessionManager) watch(id string, w wrapper.Wrapper) {
	_ = w.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok || session.wrapper != w {
		return
	}

	session.Status = SessionStatusExited
	session.wrapper = nil
}

func sortSessions(sessions []Session) {
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

func newUUID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}

	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80

	var encoded [32]byte
	hex.Encode(encoded[:], data[:])

	var id [36]byte
	copy(id[0:8], encoded[0:8])
	id[8] = '-'
	copy(id[9:13], encoded[8:12])
	id[13] = '-'
	copy(id[14:18], encoded[12:16])
	id[18] = '-'
	copy(id[19:23], encoded[16:20])
	id[23] = '-'
	copy(id[24:36], encoded[20:32])

	return string(id[:]), nil
}
