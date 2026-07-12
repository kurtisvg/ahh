package sessions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/wrapper"
)

func TestManagerCreateRequiresName(t *testing.T) {
	manager := newTestManager(t, func(context.Context) (wrapper.Wrapper, error) {
		t.Fatal("wrapper should not start without a session name")
		return nil, nil
	})

	_, err := manager.Create(t.Context(), " \t\n ")
	if err == nil {
		t.Fatal("Create() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "session name is required") {
		t.Fatalf("Create() error = %q, want containing session name is required", err.Error())
	}
}

func TestManagerCreateTrimsName(t *testing.T) {
	fake := newTestWrapper()
	manager := newTestManager(t, func(context.Context) (wrapper.Wrapper, error) {
		return fake, nil
	})

	session, err := manager.Create(t.Context(), "  terminal  ")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer fake.shutdown()

	if got := session.Metadata().Name; got != "terminal" {
		t.Fatalf("Create() name = %q, want terminal", got)
	}
}

func TestManagerCreateListsOnlyStartedSessions(t *testing.T) {
	fake := newTestWrapper()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseStart := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	manager := newTestManager(t, func(context.Context) (wrapper.Wrapper, error) {
		close(started)
		<-release
		return fake, nil
	})
	defer releaseStart()
	defer fake.shutdown()

	type createResult struct {
		session *Session
		err     error
	}
	result := make(chan createResult, 1)
	go func() {
		session, err := manager.Create(t.Context(), "terminal")
		result <- createResult{session: session, err: err}
	}()

	<-started
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("List() length during wrapper startup = %d, want 0", len(got))
	}

	releaseStart()
	got := <-result
	if got.err != nil {
		t.Fatalf("Create() error = %v", got.err)
	}
	if got.session == nil {
		t.Fatal("Create() session = nil, want session")
	}
}

func TestManagerListSortsMostRecentFirst(t *testing.T) {
	var wrappers []*testWrapper
	startWrapper := func(context.Context) (wrapper.Wrapper, error) {
		fake := newTestWrapper()
		wrappers = append(wrappers, fake)
		return fake, nil
	}
	defer func() {
		for _, fake := range wrappers {
			fake.shutdown()
		}
	}()

	base := time.Date(2026, time.July, 7, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		base,
		base.Add(time.Minute),
		base.Add(2 * time.Minute),
	}
	clockIndex := 0
	now := func() time.Time {
		t := times[clockIndex]
		clockIndex++
		return t
	}

	nextID := 0
	newID := func() (string, error) {
		nextID++
		return fmt.Sprintf("session-%d", nextID), nil
	}

	manager, err := NewManager(
		WithStartWrapper(startWrapper),
		WithClock(now),
		WithIDGenerator(newID),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	first, err := manager.Create(t.Context(), "first")
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := manager.Create(t.Context(), "second")
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	first.Touch()

	got := manager.List()
	if len(got) != 2 {
		t.Fatalf("List() length = %d, want 2", len(got))
	}
	if got[0].ID != first.ID() || got[1].ID != second.ID() {
		t.Fatalf(
			"List() order = [%q, %q], want [%q, %q]",
			got[0].ID,
			got[1].ID,
			first.ID(),
			second.ID(),
		)
	}
}

func TestManagerDeleteKeepsSessionWhenShutdownFails(t *testing.T) {
	fake := newTestWrapper()
	fake.shutdownErr = errors.New("boom")
	manager := newTestManager(t, func(context.Context) (wrapper.Wrapper, error) {
		return fake, nil
	})

	session, err := manager.Create(t.Context(), "terminal")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer fake.shutdown()

	deleted, err := manager.Delete(t.Context(), session.ID())
	if !deleted {
		t.Fatal("Delete() deleted = false, want true")
	}
	if err == nil {
		t.Fatal("Delete() error = nil, want error")
	}

	if _, ok := manager.Get(session.ID()); !ok {
		t.Fatal("Delete() removed session after shutdown failure")
	}
}

func TestNewSessionIDGeneratesUUIDV4(t *testing.T) {
	id, err := newSessionID()
	if err != nil {
		t.Fatalf("newSessionID() error = %v", err)
	}

	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("parse session id %q: %v", id, err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("session id version = %d, want 4", parsed.Version())
	}
	if parsed.Variant() != uuid.RFC4122 {
		t.Fatalf("session id variant = %v, want RFC4122", parsed.Variant())
	}
}

type testWrapper struct {
	done        chan struct{}
	closeOnce   sync.Once
	shutdownErr error
}

func newTestWrapper() *testWrapper {
	return &testWrapper{
		done: make(chan struct{}),
	}
}

func (w *testWrapper) Address() string {
	return "127.0.0.1:1"
}

func (w *testWrapper) Wait() error {
	<-w.done

	return nil
}

func (w *testWrapper) Shutdown(context.Context) error {
	if w.shutdownErr != nil {
		return w.shutdownErr
	}

	w.shutdown()

	return nil
}

func (w *testWrapper) shutdown() {
	w.closeOnce.Do(func() {
		close(w.done)
	})
}

func newTestManager(
	t *testing.T,
	startWrapper func(context.Context) (wrapper.Wrapper, error),
) *Manager {
	t.Helper()

	manager, err := NewManager(WithStartWrapper(startWrapper))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	return manager
}
