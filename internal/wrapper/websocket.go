package wrapper

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// inputMessage carries input, a resize, or a rendered output acknowledgment.
type inputMessage struct {
	Type   string `json:"type"`
	Data   string `json:"data,omitempty"`
	Rows   uint16 `json:"rows,omitempty"`
	Cols   uint16 `json:"cols,omitempty"`
	Offset uint64 `json:"offset,string,omitempty"`
}

// replayMessage announces where output begins and whether earlier bytes were lost.
type replayMessage struct {
	Type   string `json:"type"`
	Offset string `json:"offset"`
	Gap    bool   `json:"gap"`
}

// servePTY streams output from an optional decimal ?cursor=N byte offset.
// It sends a replay message first, then binary chunks prefixed by an eight-byte
// big-endian offset. Clients acknowledge the next rendered byte with
// {"type":"ack","offset":"N"}; decimal strings preserve full uint64 precision.
func (s *Server) servePTY(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.socketsMu.Lock()
		if ctx.Err() != nil || s.draining {
			s.socketsMu.Unlock()
			http.Error(w, "wrapper is shutting down", http.StatusServiceUnavailable)
			return
		}
		s.sockets.Add(1)
		s.socketsMu.Unlock()
		defer s.sockets.Done()

		var cursor *uint64
		if value := r.URL.Query().Get("cursor"); value != "" {
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
			cursor = &n
		}
		state, err := s.terminal.attach(cursor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		defer s.terminal.detach(state.client)

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		// Bound incoming message size; larger pastes must be split across messages.
		conn.SetReadLimit(64 * 1024) // 64 KiB

		// Wrapper shutdown must also stop this WebSocket handler.
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initial, stop := context.WithTimeout(ctx, 5*time.Second)
		err = wsjson.Write(initial, conn, replayMessage{
			Type:   "replay",
			Offset: strconv.FormatUint(state.history.offset, 10),
			Gap:    state.gap,
		})
		stop()
		if err != nil {
			return
		}

		// Read acknowledgments while sending replay and live output.
		done := make(chan error, 3)
		go func() { done <- sendOutput(ctx, conn, state.client, state.history) }()
		go func() { done <- s.readInput(ctx, conn, state.client) }()
		// Detect idle connections whose browser is no longer reachable.
		go func() { done <- heartbeat(ctx, conn) }()

		// Join I/O and heartbeat goroutines before removing the attachment.
		<-done
		cancel()
		_ = conn.CloseNow()
		<-done
		<-done
	}
}

// heartbeat detects silent connections that no longer respond to WebSocket pings.
func heartbeat(ctx context.Context, conn *websocket.Conn) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ping, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(ping)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

// sendOutput limits unacknowledged bytes so a slow browser cannot buffer forever.
func sendOutput(ctx context.Context, conn *websocket.Conn, client *attachment, history outputFrame) error {
	acked, sent := history.offset, history.offset
	acknowledge := func(offset uint64) error {
		if offset < acked || offset > sent {
			return errors.New("invalid output acknowledgment")
		}
		acked = offset
		return nil
	}

	write := func(frame outputFrame) error {
		end := frame.offset + uint64(len(frame.data))
		wait, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		// Allow a small burst while the browser renders; 128 KiB is an initial budget.
		for end-acked > 128*1024 {
			select {
			case offset := <-client.ack:
				if err := acknowledge(offset); err != nil {
					return err
				}
			case <-wait.Done():
				return errors.New("terminal reader stalled")
			}
		}

		data := binary.BigEndian.AppendUint64(nil, frame.offset)
		data = append(data, frame.data...)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
			return err
		}
		sent = end
		return nil
	}

	// Send saved output before queued live output.
	for len(history.data) > 0 {
		n := min(len(history.data), 32*1024)
		if err := write(outputFrame{offset: history.offset, data: history.data[:n]}); err != nil {
			return err
		}
		history.offset += uint64(n)
		history.data = history.data[n:]
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case offset := <-client.ack:
			if err := acknowledge(offset); err != nil {
				return err
			}
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

// readInput accepts acknowledgments, input, and resize commands from any connection.
func (s *Server) readInput(ctx context.Context, conn *websocket.Conn, client *attachment) error {
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

		if msg.Type == "ack" {
			select {
			case client.ack <- msg.Offset:
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
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
