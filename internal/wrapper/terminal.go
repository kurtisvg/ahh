package wrapper

import (
	"fmt"
	"io"
	"sync"

	"github.com/kurtisvg/ahh/internal/harness"
)

// terminalHistoryLimit caps replay data sent to newly connected browsers.
const terminalHistoryLimit = 1 << 20

// terminalSession fans PTY output out to browser clients while keeping enough
// recent output to make reconnects and late browser opens useful.
type terminalSession struct {
	harness harness.Harness

	// mu protects history, clients, and closed. PTY reads happen in one goroutine;
	// browser writes and resizes call directly into the harness.
	mu      sync.Mutex
	history []byte
	clients map[chan []byte]struct{}
	closed  bool

	// done is closed after the PTY reader exits and all browser output channels
	// are closed.
	done      chan struct{}
	closeOnce sync.Once
}

func newTerminalSession(h harness.Harness) *terminalSession {
	session := &terminalSession{
		harness: h,
		clients: map[chan []byte]struct{}{},
		done:    make(chan struct{}),
	}

	go session.readPTY()

	return session
}

func (s *terminalSession) subscribe() (chan []byte, []byte, bool) {
	ch := make(chan []byte, 64)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		close(ch)
		return ch, nil, false
	}

	s.clients[ch] = struct{}{}
	// Copy the replay buffer so future PTY output cannot mutate data already
	// handed to this websocket connection.
	replay := append([]byte(nil), s.history...)

	return ch, replay, true
}

func (s *terminalSession) unsubscribe(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[ch]; !ok {
		return
	}

	delete(s.clients, ch)
	close(ch)
}

func (s *terminalSession) WriteString(data string) error {
	if _, err := io.WriteString(s.harness, data); err != nil {
		return fmt.Errorf("write terminal input: %w", err)
	}

	return nil
}

func (s *terminalSession) Resize(rows, cols uint16) error {
	if err := s.harness.Resize(rows, cols); err != nil {
		return fmt.Errorf("resize terminal: %w", err)
	}

	return nil
}

func (s *terminalSession) Close() {
	s.closeOnce.Do(func() {
		s.harness.Close()
		<-s.done
	})
}

func (s *terminalSession) readPTY() {
	defer close(s.done)

	buf := make([]byte, 32*1024)
	for {
		n, err := s.harness.Read(buf)
		if n > 0 {
			s.publish(buf[:n])
		}
		if err != nil {
			s.closeClients()
			return
		}
	}
}

func (s *terminalSession) publish(data []byte) {
	output := append([]byte(nil), data...)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.history = append(s.history, output...)
	if len(s.history) > terminalHistoryLimit {
		s.history = append([]byte(nil), s.history[len(s.history)-terminalHistoryLimit:]...)
	}

	for ch := range s.clients {
		select {
		case ch <- output:
		default:
			// Do not let one slow browser block the harness reader. That client
			// can reconnect and receive the bounded replay buffer.
		}
	}
}

func (s *terminalSession) closeClients() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	for ch := range s.clients {
		delete(s.clients, ch)
		close(ch)
	}
}
