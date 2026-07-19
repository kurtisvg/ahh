package conversations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurtisvg/ahh/internal/server/agents"
	"github.com/kurtisvg/ahh/internal/wrapper"
)

const testAgentID = "claude-code"

func TestManagerCreateRequiresName(t *testing.T) {
	manager := newTestManager(t, func(context.Context, WrapperStart) (wrapper.Wrapper, error) {
		t.Fatal("wrapper should not start without a conversation name")
		return nil, nil
	})

	_, err := manager.Create(t.Context(), " \t\n ", testAgentID)
	if err == nil {
		t.Fatal("Create() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "conversation name is required") {
		t.Fatalf("Create() error = %q, want containing conversation name is required", err.Error())
	}
}

func TestManagerCreateTrimsName(t *testing.T) {
	fake := newTestWrapper()
	var gotStart WrapperStart
	manager := newTestManager(t, func(_ context.Context, start WrapperStart) (wrapper.Wrapper, error) {
		gotStart = start
		return fake, nil
	})

	conversation, err := manager.Create(t.Context(), "  terminal  ", testAgentID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer fake.shutdown()

	if got := conversation.Metadata().Name; got != "terminal" {
		t.Fatalf("Create() name = %q, want terminal", got)
	}
	if got := conversation.Metadata().AgentID; got != testAgentID {
		t.Fatalf("Create() AgentID = %q, want %q", got, testAgentID)
	}
	if gotStart.Harness != agents.ClaudeCodeHarness || gotStart.ConfigDir != "/test/agents/claude-code/config" {
		t.Fatalf("wrapper start = %+v, want resolved Claude harness and config directory", gotStart)
	}
}

func TestNewManagerRequiresAgentResolver(t *testing.T) {
	if _, err := NewManager(t.Context()); err == nil || !strings.Contains(err.Error(), "agent resolver is required") {
		t.Fatalf("NewManager() error = %v, want required Agent resolver", err)
	}
}

func TestManagerCreateRequiresExistingAgent(t *testing.T) {
	manager := newTestManager(t, func(context.Context, WrapperStart) (wrapper.Wrapper, error) {
		t.Fatal("wrapper should not start without an existing Agent")
		return nil, nil
	})

	if _, err := manager.Create(t.Context(), "terminal", ""); !errors.Is(err, ErrAgentRequired) {
		t.Fatalf("Create(empty Agent) error = %v, want ErrAgentRequired", err)
	}
	if _, err := manager.Create(t.Context(), "terminal", "missing"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("Create(missing Agent) error = %v, want ErrAgentNotFound", err)
	}
}

func TestManagerCreateUsesManagerLifecycleContext(t *testing.T) {
	fake := newTestWrapper()
	var wrapperCtx context.Context
	lifecycleCtx, cancelLifecycle := context.WithCancel(t.Context())
	manager, err := NewManager(
		lifecycleCtx,
		WithAgentResolver(newTestAgentResolver()),
		WithStartWrapper(func(ctx context.Context, _ WrapperStart) (wrapper.Wrapper, error) {
			wrapperCtx = ctx
			return fake, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer fake.shutdown()

	requestCtx, cancelRequest := context.WithCancel(t.Context())
	if _, err := manager.Create(requestCtx, "terminal", testAgentID); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	cancelRequest()

	if err := wrapperCtx.Err(); err != nil {
		t.Fatalf("wrapper context error after request cancellation = %v, want nil", err)
	}

	cancelLifecycle()
	if err := wrapperCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("wrapper context error after lifecycle cancellation = %v, want context canceled", err)
	}
}

func TestManagerShutdownCancelsLifecycleAndRejectsCreate(t *testing.T) {
	fake := newTestWrapper()
	var wrapperCtx context.Context
	manager := newTestManager(t, func(ctx context.Context, _ WrapperStart) (wrapper.Wrapper, error) {
		wrapperCtx = ctx
		return fake, nil
	})

	if _, err := manager.Create(t.Context(), "terminal", testAgentID); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := manager.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := wrapperCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("wrapper context error after shutdown = %v, want context canceled", err)
	}
	if _, err := manager.Create(t.Context(), "another", testAgentID); err == nil {
		t.Fatal("Create() error after shutdown = nil, want error")
	}
}

func TestManagerCreateListsOnlyStartedConversations(t *testing.T) {
	fake := newTestWrapper()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseStart := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	manager := newTestManager(t, func(context.Context, WrapperStart) (wrapper.Wrapper, error) {
		close(started)
		<-release
		return fake, nil
	})
	defer releaseStart()
	defer fake.shutdown()

	type createResult struct {
		conversation *Conversation
		err          error
	}
	result := make(chan createResult, 1)
	go func() {
		conversation, err := manager.Create(t.Context(), "terminal", testAgentID)
		result <- createResult{conversation: conversation, err: err}
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
	if got.conversation == nil {
		t.Fatal("Create() conversation = nil, want conversation")
	}
}

func TestManagerCreateDoesNotRegisterAfterShutdown(t *testing.T) {
	fake := newTestWrapper()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseStart := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	manager := newTestManager(t, func(context.Context, WrapperStart) (wrapper.Wrapper, error) {
		close(started)
		<-release
		return fake, nil
	})
	defer releaseStart()

	result := make(chan error, 1)
	go func() {
		_, err := manager.Create(t.Context(), "terminal", testAgentID)
		result <- err
	}()

	<-started
	if err := manager.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	releaseStart()
	if err := <-result; err == nil {
		t.Fatal("Create() error = nil, want shutdown error")
	}
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("List() length after shutdown during create = %d, want 0", len(got))
	}
	select {
	case <-fake.done:
	default:
		t.Fatal("wrapper was not shut down after concurrent manager shutdown")
	}
}

func TestConversationStartDeduplicatesConcurrentCalls(t *testing.T) {
	fake := newTestWrapper()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls int
	var callsMu sync.Mutex
	var startedOnce sync.Once
	conversation := restoreConversation(
		Metadata{ID: "persisted", AgentID: testAgentID, Name: "persisted"},
		newTestAgentResolver(),
		time.Now,
		nil,
		func(WrapperStart) (wrapper.Wrapper, error) {
			callsMu.Lock()
			calls++
			callsMu.Unlock()
			startedOnce.Do(func() {
				close(started)
			})
			<-release
			return fake, nil
		},
	)

	results := make(chan wrapper.Wrapper, 2)
	for range 2 {
		go func() {
			w, err := conversation.Start(t.Context())
			if err != nil {
				t.Errorf("Start() error = %v", err)
			}
			results <- w
		}()
	}

	<-started
	close(release)
	first := <-results
	second := <-results
	if first != fake || second != fake {
		t.Fatalf("Start() wrappers = [%p, %p], want %p", first, second, fake)
	}
	callsMu.Lock()
	gotCalls := calls
	callsMu.Unlock()
	if gotCalls != 1 {
		t.Fatalf("wrapper start calls = %d, want 1", gotCalls)
	}
	if err := conversation.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestConversationDeletePreventsRestartAndMetadataWrites(t *testing.T) {
	store := &testMetadataStore{}
	startCalled := false
	conversation := restoreConversation(
		Metadata{ID: "persisted", AgentID: testAgentID, Name: "persisted"},
		newTestAgentResolver(),
		time.Now,
		store,
		func(WrapperStart) (wrapper.Wrapper, error) {
			startCalled = true
			return newTestWrapper(), nil
		},
	)

	if err := conversation.delete(t.Context()); err != nil {
		t.Fatalf("delete() error = %v", err)
	}
	if store.deletedID != "persisted" {
		t.Fatalf("deleted conversation ID = %q, want persisted", store.deletedID)
	}
	if err := conversation.Touch(); err == nil {
		t.Fatal("Touch() error after delete = nil, want error")
	}
	if store.saveCalls != 0 {
		t.Fatalf("metadata saves after delete = %d, want 0", store.saveCalls)
	}
	if _, err := conversation.Start(t.Context()); err == nil {
		t.Fatal("Start() error after delete = nil, want error")
	}
	if startCalled {
		t.Fatal("wrapper started after conversation delete")
	}
}

func TestConversationMarkResumablePersistsOnce(t *testing.T) {
	store := &testMetadataStore{}
	conversation := restoreConversation(
		Metadata{ID: "persisted", AgentID: testAgentID, Name: "persisted"},
		newTestAgentResolver(),
		time.Now,
		store,
		func(WrapperStart) (wrapper.Wrapper, error) {
			return newTestWrapper(), nil
		},
	)

	if err := conversation.MarkResumable(); err != nil {
		t.Fatalf("MarkResumable() error = %v", err)
	}
	if err := conversation.MarkResumable(); err != nil {
		t.Fatalf("second MarkResumable() error = %v", err)
	}
	if !conversation.Metadata().Resumable {
		t.Fatal("conversation resumable = false, want true")
	}
	if store.saveCalls != 1 {
		t.Fatalf("metadata saves = %d, want 1", store.saveCalls)
	}
}

func TestUntouchedRestoredConversationStartsWithoutResume(t *testing.T) {
	fake := newTestWrapper()
	var gotStart WrapperStart
	conversation := restoreConversation(
		Metadata{ID: "persisted", AgentID: testAgentID, Name: "persisted"},
		newTestAgentResolver(),
		time.Now,
		nil,
		func(start WrapperStart) (wrapper.Wrapper, error) {
			gotStart = start
			return fake, nil
		},
	)

	if _, err := conversation.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if gotStart.Resume {
		t.Fatalf("wrapper start = %+v, want new conversation", gotStart)
	}
	if err := conversation.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestRestoredConversationResolvesCurrentAgentAtStart(t *testing.T) {
	fake := newTestWrapper()
	resolver := newTestAgentResolver()
	var gotStart WrapperStart
	conversation := restoreConversation(
		Metadata{ID: "persisted", AgentID: testAgentID, Name: "persisted"},
		resolver,
		time.Now,
		nil,
		func(start WrapperStart) (wrapper.Wrapper, error) {
			gotStart = start
			return fake, nil
		},
	)
	resolver.configDir = "/test/agents/claude-code/current-config"

	if _, err := conversation.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if gotStart.ConfigDir != resolver.configDir {
		t.Fatalf("Start() ConfigDir = %q, want current %q", gotStart.ConfigDir, resolver.configDir)
	}
	if err := conversation.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestRestoredConversationWithMissingAgentStaysExited(t *testing.T) {
	resolver := newTestAgentResolver()
	conversation := restoreConversation(
		Metadata{ID: "persisted", AgentID: testAgentID, Name: "persisted"},
		resolver,
		time.Now,
		nil,
		func(WrapperStart) (wrapper.Wrapper, error) {
			t.Fatal("wrapper should not start when its Agent is missing")
			return nil, nil
		},
	)
	delete(resolver.agents, testAgentID)

	if _, err := conversation.Start(t.Context()); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("Start() error = %v, want ErrAgentNotFound", err)
	}
	if _, status := conversation.Wrapper(); status != StatusExited {
		t.Fatalf("status = %q, want %q", status, StatusExited)
	}
}

func TestManagerListSortsMostRecentFirst(t *testing.T) {
	var wrappers []*testWrapper
	startWrapper := func(context.Context, WrapperStart) (wrapper.Wrapper, error) {
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
		return fmt.Sprintf("conversation-%d", nextID), nil
	}

	manager, err := NewManager(
		t.Context(),
		WithAgentResolver(newTestAgentResolver()),
		WithStartWrapper(startWrapper),
		WithClock(now),
		WithIDGenerator(newID),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	first, err := manager.Create(t.Context(), "first", testAgentID)
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := manager.Create(t.Context(), "second", testAgentID)
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	if err := first.Touch(); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}

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

func TestManagerDeleteKeepsConversationWhenShutdownFails(t *testing.T) {
	fake := newTestWrapper()
	fake.shutdownErr = errors.New("boom")
	manager := newTestManager(t, func(context.Context, WrapperStart) (wrapper.Wrapper, error) {
		return fake, nil
	})

	conversation, err := manager.Create(t.Context(), "terminal", testAgentID)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer fake.shutdown()

	deleted, err := manager.Delete(t.Context(), conversation.ID())
	if !deleted {
		t.Fatal("Delete() deleted = false, want true")
	}
	if err == nil {
		t.Fatal("Delete() error = nil, want error")
	}

	if _, ok := manager.Get(conversation.ID()); !ok {
		t.Fatal("Delete() removed conversation after shutdown failure")
	}
}

func TestNewConversationIDGeneratesUUIDV4(t *testing.T) {
	id, err := newConversationID()
	if err != nil {
		t.Fatalf("newConversationID() error = %v", err)
	}

	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("parse conversation id %q: %v", id, err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("conversation id version = %d, want 4", parsed.Version())
	}
	if parsed.Variant() != uuid.RFC4122 {
		t.Fatalf("conversation id variant = %v, want RFC4122", parsed.Variant())
	}
}

type testWrapper struct {
	done        chan struct{}
	closeOnce   sync.Once
	shutdownErr error
}

type testMetadataStore struct {
	saveCalls int
	deletedID string
}

type testAgentResolver struct {
	agents    map[string]agents.Config
	configDir string
}

func newTestAgentResolver() *testAgentResolver {
	return &testAgentResolver{
		agents: map[string]agents.Config{
			testAgentID: {
				ID:      testAgentID,
				Name:    "Claude Code",
				Harness: agents.ClaudeCodeHarness,
			},
		},
		configDir: "/test/agents/claude-code/config",
	}
}

func (r *testAgentResolver) Get(id string) (agents.Config, bool) {
	agent, ok := r.agents[id]
	return agent, ok
}

func (r *testAgentResolver) ConfigDir(id string) (string, error) {
	if _, ok := r.agents[id]; !ok {
		return "", agents.ErrNotFound
	}
	return r.configDir, nil
}

func (s *testMetadataStore) Load() ([]Metadata, error) {
	return nil, nil
}

func (s *testMetadataStore) Save(Metadata) error {
	s.saveCalls++
	return nil
}

func (s *testMetadataStore) Delete(id string) error {
	s.deletedID = id
	return nil
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
	startWrapper func(context.Context, WrapperStart) (wrapper.Wrapper, error),
) *Manager {
	t.Helper()

	manager, err := NewManager(
		t.Context(),
		WithAgentResolver(newTestAgentResolver()),
		WithStartWrapper(startWrapper),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	return manager
}
