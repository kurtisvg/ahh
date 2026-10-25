package wrapper

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

type inputMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

func (s *Server) servePTY(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.socketsMu.Lock()
		if ctx.Err() != nil {
			s.socketsMu.Unlock()
			http.Error(w, "wrapper is shutting down", http.StatusServiceUnavailable)
			return
		}
		s.sockets.Add(1)
		s.socketsMu.Unlock()
		defer s.sockets.Done()

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		// Bound incoming message size; larger pastes must be split across messages.
		conn.SetReadLimit(64 * 1024) // 64 KiB

		client, history := s.terminal.attach()
		defer s.terminal.detach(client)

		// Wrapper shutdown must also stop this WebSocket handler.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		done := make(chan error, 2)
		go func() { done <- sendOutput(ctx, conn, client, history) }()
		go func() { done <- s.readInput(ctx, conn) }()

		// Stop both I/O goroutines before removing the attachment.
		<-done
		cancel()
		_ = conn.CloseNow()
		<-done
	}
}

func sendOutput(ctx context.Context, conn *websocket.Conn, client *attachment, history []byte) error {
	write := func(data []byte) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return conn.Write(ctx, websocket.MessageBinary, data)
	}

	// Send saved output before queued live output.
	for len(history) > 0 {
		n := min(len(history), 32<<10)
		if err := write(history[:n]); err != nil {
			return err
		}
		history = history[n:]
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data, ok := <-client.output:
			if !ok {
				return nil
			}
			if err := write(data); err != nil {
				return err
			}
		}
	}
}

// readInput accepts input and resize commands from any connection.
func (s *Server) readInput(ctx context.Context, conn *websocket.Conn) error {
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if kind != websocket.MessageText {
			return errors.New("expected a JSON text message")
		}

		var msg inputMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return err
		}

		if err := s.applyInput(ctx, msg); err != nil {
			return err
		}
	}
}

// applyInput serializes harness writes and resizes; the last resize takes effect.
func (s *Server) applyInput(ctx context.Context, msg inputMessage) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()

	// Skip queued commands if the handler was canceled.
	if err := ctx.Err(); err != nil {
		return err
	}

	switch msg.Type {
	case "input":
		_, err := io.WriteString(s.harness, msg.Data)
		return err
	case "resize":
		if msg.Rows == 0 || msg.Cols == 0 || msg.Rows > 1000 || msg.Cols > 1000 {
			return errors.New("invalid terminal size")
		}
		return s.harness.Resize(msg.Rows, msg.Cols)
	default:
		return errors.New("unknown terminal message")
	}
}
