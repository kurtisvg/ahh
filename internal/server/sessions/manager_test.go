package sessions

import (
	"context"
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

	manager, err := New(
		WithStartWrapper(startWrapper),
		WithClock(now),
		WithIDGenerator(newID),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
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
	done      chan struct{}
	closeOnce sync.Once
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

	manager, err := New(WithStartWrapper(startWrapper))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return manager
}
