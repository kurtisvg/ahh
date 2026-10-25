package wrapper

import "sync"

// replayLimit caps saved output, dropping the oldest bytes when full.
// The 1 MiB default is a starting point, not based on measured usage.
const replayLimit = 1024 * 1024 // 1 MiB

// terminal stores recent output and each attachment's queue.
type terminal struct {
	mu      sync.Mutex
	history []byte
	clients map[*attachment]bool
	closed  bool
}

// attachment holds the output queue for one browser connection.
type attachment struct {
	output chan []byte
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

	chunk := append([]byte(nil), data...)
	for client := range t.clients {
		select {
		case client.output <- chunk:
		default:
			// Drop slow readers instead of blocking harness output.
			close(client.output)
			delete(t.clients, client)
		}
	}

	return len(data), nil
}

// attach returns a new attachment and saved output.
func (t *terminal) attach() (*attachment, []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()

	client := &attachment{
		output: make(chan []byte, 16),
	}
	if t.closed {
		close(client.output)
	} else {
		t.clients[client] = true
	}

	// Subscribe and copy history under one lock so no output is missed.
	return client, append([]byte(nil), t.history...)
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
	for client := range t.clients {
		close(client.output)
		delete(t.clients, client)
	}
}
