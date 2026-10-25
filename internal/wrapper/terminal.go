package wrapper

import (
	"errors"
	"sync"
)

// replayLimit caps saved output, dropping the oldest bytes when full.
// The 1 MiB default is a starting point, not based on measured usage.
const replayLimit = 1024 * 1024 // 1 MiB

// terminal stores recent output and each attachment's queue.
type terminal struct {
	mu      sync.Mutex
	history []byte

	// nextOffset counts all output bytes, including those dropped from history.
	nextOffset uint64

	// Closed queues stay registered until their handlers finish.
	clients map[*attachment]bool
	closed  bool
}

// attachment holds output and acknowledgment queues for one browser connection.
type attachment struct {
	output chan outputFrame
	ack    chan uint64
}

// outputFrame identifies the first byte in an immutable chunk of output.
type outputFrame struct {
	offset uint64
	data   []byte
}

// attachmentState pairs a new subscription with its replay snapshot.
type attachmentState struct {
	client  *attachment
	history outputFrame
	gap     bool
}

// newTerminal creates a terminal ready to accept output and attachments.
func newTerminal() *terminal {
	return &terminal{
		clients: make(map[*attachment]bool),
	}
}

// Write saves recent output and queues it for each connected browser.
func (t *terminal) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.history = append(t.history, data...)
	if len(t.history) > replayLimit {
		// Keep the newest bytes in a fresh buffer so the larger buffer can be freed.
		t.history = append([]byte(nil), t.history[len(t.history)-replayLimit:]...)
	}

	chunk := outputFrame{
		offset: t.nextOffset,
		data:   append([]byte(nil), data...),
	}
	t.nextOffset += uint64(len(data))
	for client, active := range t.clients {
		if !active {
			continue
		}
		select {
		case client.output <- chunk:
		default:
			// Drop slow readers instead of blocking harness output.
			close(client.output)
			t.clients[client] = false
		}
	}

	return len(data), nil
}

// attach subscribes at cursor, or at the oldest retained byte when cursor is nil.
func (t *terminal) attach(cursor *uint64) (attachmentState, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	oldest := t.nextOffset - uint64(len(t.history))
	start := oldest
	gap := oldest > 0
	if cursor != nil {
		if *cursor > t.nextOffset {
			return attachmentState{}, errors.New("cursor ahead of output")
		}
		gap = *cursor < oldest
		start = max(oldest, *cursor)
	}

	// Bound per-connection snapshots and queues, including handlers still closing.
	if len(t.clients) >= 16 {
		return attachmentState{}, errors.New("too many attachments")
	}
	client := &attachment{
		output: make(chan outputFrame, 16),
		ack:    make(chan uint64, 16),
	}
	t.clients[client] = !t.closed
	if t.closed {
		close(client.output)
	}

	// Subscribe and copy history under one lock so no output is missed.
	return attachmentState{
		client: client,
		history: outputFrame{
			offset: start,
			data:   append([]byte(nil), t.history[start-oldest:]...),
		},
		gap: gap,
	}, nil
}

// detach removes an attachment from the output broadcast.
func (t *terminal) detach(client *attachment) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.clients, client)
}

// finish closes output queues; handlers send queued bytes before disconnecting.
func (t *terminal) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	for client, active := range t.clients {
		if active {
			close(client.output)
		}
		t.clients[client] = false
	}
}
