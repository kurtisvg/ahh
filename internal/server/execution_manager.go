package server

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
)

// executionManager coordinates local wrapper execution. Its methods are safe to call
// concurrently; process launch and cleanup happen outside the bookkeeping lock.
type executionManager struct {
	mu     sync.Mutex
	states map[string]*executionState
	launch func(context.Context, string) (*wrapperProcess, error)
	closed bool
}

// executionState holds the current invocation and its runtime coordination.
// All fields are protected by executionManager.mu.
type executionState struct {
	process      *wrapperProcess
	invocationID string
	starting     chan struct{}
	cancel       context.CancelFunc
	startErr     error
	stopDone     chan struct{}
	stopErr      error
}

// The launcher must honor cancellation and finish failed startup cleanup before
// returning. A process returned with an error retains uncertain ownership.
func newExecutionManager(
	launch func(context.Context, string) (*wrapperProcess, error),
) *executionManager {
	return &executionManager{
		states: make(map[string]*executionState),
		launch: launch,
	}
}

// Start begins a launch unless one is already starting or running. Execution
// outlives the request; Stop or Close owns its cancellation.
func (m *executionManager) Start(ctx context.Context, id, directory string) error {
	_, err := m.start(ctx, id, directory)
	return err
}

func (m *executionManager) start(
	ctx context.Context,
	id, directory string,
) (*executionState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.closed {
		return nil, errors.New("execution manager is closed")
	}
	state := m.states[id]
	if state == nil {
		state = &executionState{}
	}
	if state.stopDone != nil {
		select {
		case <-state.stopDone:
		default:
			return nil, errors.New("conversation is stopping")
		}
	}
	if state.starting != nil {
		return state, nil
	}
	if state.process != nil {
		select {
		case <-state.process.done:
			if state.process.result == nil {
				return nil, errors.New("previous harness exit is unconfirmed")
			}
		default:
			return state, nil
		}
	}
	if state.cancel != nil {
		state.cancel()
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	state = &executionState{
		invocationID: rand.Text(),
		starting:     make(chan struct{}),
		cancel:       cancel,
	}
	m.states[id] = state
	go func() {
		process, err := m.launch(ctx, directory)
		m.mu.Lock()
		defer m.mu.Unlock()
		state.process, state.startErr = process, err
		if err != nil {
			state.cancel()
		}
		close(state.starting)
		state.starting = nil
	}()
	return state, nil
}

// Status reads runtime state without registering an unused conversation.
func (m *executionManager) Status(id string) processStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := processStatus{State: "stopped"}
	state := m.states[id]
	if state == nil {
		return status
	}
	status.Invocation = state.invocationID
	if state.stopDone != nil {
		select {
		case <-state.stopDone:
			if state.stopErr != nil {
				status.State = "failed"
				status.Error = "Harness exit is unconfirmed. Check the server logs."
			}
		default:
			status.State = "stopping"
		}
		return status
	}
	if state.starting != nil {
		status.State = "starting"
		return status
	}
	if state.startErr != nil {
		status.Error = "Could not start terminal. Check the server logs and retry."
	}
	if state.process == nil {
		return status
	}
	status.State = "running"
	select {
	case <-state.process.done:
		status.State = "failed"
		if result := state.process.result; result != nil {
			status.State = "exited"
			status.ExitCode, status.Signal = result.ExitCode, result.Signal
		}
	default:
	}
	return status
}

// Process resolves one specific invocation. Existing attachments keep this
// concrete process reference and cannot be redirected by a later launch.
func (m *executionManager) Process(id, invocationID string) (*wrapperProcess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.states[id]
	if state == nil || state.invocationID != invocationID || state.process == nil {
		return nil, errors.New("conversation invocation is unavailable")
	}
	return state.process, nil
}

// Stop ends the current invocation and joins cleanup. A later Start may launch
// again only after the previous harness's exit has been confirmed.
func (m *executionManager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	state := m.states[id]
	if state == nil {
		m.mu.Unlock()
		return nil
	}
	if state.stopDone != nil {
		done := state.stopDone
		m.mu.Unlock()
		<-done
		m.mu.Lock()
		defer m.mu.Unlock()
		return state.stopErr
	}
	state.stopDone = make(chan struct{})
	state.cancel()
	starting := state.starting
	m.mu.Unlock()
	if starting != nil {
		// Retain ownership until the canceled launcher has finished cleanup.
		<-starting
	}
	m.mu.Lock()
	process := state.process
	m.mu.Unlock()
	var err error
	if process != nil {
		_ = process.Shutdown(ctx)
		_ = process.Wait()
		if process.result == nil {
			err = errors.New("harness exit is unconfirmed")
		}
	}
	m.mu.Lock()
	state.stopErr = err
	close(state.stopDone)
	m.mu.Unlock()
	return err
}

// Close prevents new launches and joins every conversation's cleanup.
func (m *executionManager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	ids := make([]string, 0, len(m.states))
	for id := range m.states {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	var jobs sync.WaitGroup
	errorsFound := make(chan error, len(ids))
	for _, id := range ids {
		jobs.Go(func() { errorsFound <- m.Stop(ctx, id) })
	}
	jobs.Wait()
	close(errorsFound)
	var result error
	for err := range errorsFound {
		result = errors.Join(result, err)
	}
	return result
}

// startAndWait preserves the transitional /start endpoint's synchronous reply.
// Canceling the wait does not cancel the conversation's execution.
func (m *executionManager) startAndWait(ctx context.Context, id, directory string) error {
	state, err := m.start(ctx, id, directory)
	if err != nil {
		return err
	}
	m.mu.Lock()
	starting := state.starting
	m.mu.Unlock()
	if starting != nil {
		select {
		case <-starting:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return state.startErr
}
