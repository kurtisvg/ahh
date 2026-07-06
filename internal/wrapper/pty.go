package wrapper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/coder/websocket"
)

// browserMessage is the browser-to-wrapper control protocol. PTY output travels
// in binary websocket frames in the opposite direction.
type browserMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// servePTY upgrades one client websocket to the shared harness PTY. PTY output
// is sent as binary frames; terminal input and resize controls are accepted as
// JSON text frames.
//
// This is a prototype terminal transport endpoint, not the final Conversation
// API. Multiple clients may observe the same PTY, but each receives its own
// replay buffer and output channel.
func servePTY(terminal *terminalSession) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		output, replay, ok := terminal.subscribe()
		if !ok {
			_ = conn.Close(websocket.StatusGoingAway, "terminal closed")
			return
		}
		defer terminal.unsubscribe(output)

		errCh := make(chan error, 2)
		go func() {
			errCh <- copyTerminalToWebSocket(ctx, conn, replay, output)
		}()
		go func() {
			errCh <- copyWebSocketToPTY(ctx, conn, terminal)
		}()

		err = <-errCh
		cancel()

		status := websocket.StatusNormalClosure
		reason := ""
		if !normalBridgeError(err) {
			status = websocket.StatusInternalError
			reason = "terminal bridge failed"
		}
		_ = conn.Close(status, reason)
	})
}

func copyTerminalToWebSocket(
	ctx context.Context,
	conn *websocket.Conn,
	replay []byte,
	output <-chan []byte,
) error {
	// Send replay before live output so a browser refresh can show terminal
	// output that happened before the websocket connected.
	if len(replay) > 0 {
		if err := conn.Write(ctx, websocket.MessageBinary, replay); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data, ok := <-output:
			if !ok {
				return nil
			}
			if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
				return err
			}
		}
	}
}

func copyWebSocketToPTY(ctx context.Context, conn *websocket.Conn, terminal *terminalSession) error {
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if messageType != websocket.MessageText {
			continue
		}

		var msg browserMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("decode terminal message: %w", err)
		}

		switch msg.Type {
		case "input":
			if err := terminal.WriteString(msg.Data); err != nil {
				return err
			}
		case "resize":
			if err := terminal.Resize(msg.Rows, msg.Cols); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown terminal message type %q", msg.Type)
		}
	}
}

func normalBridgeError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return true
	}

	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway, websocket.StatusNoStatusRcvd:
		return true
	default:
		return false
	}
}
