package wrapper

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestTerminalExchangeAndDisconnect(t *testing.T) {
	useFakeCodex(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	server, err := Start(ctx, "127.0.0.1", 0, "codex")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		if err := server.Wait(); err != nil {
			t.Error(err)
		}
	})

	address := net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
	url := "ws://" + address + "/pty"
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()

	// Resize and input use the same harness PTY.
	if err := wsjson.Write(ctx, conn, inputMessage{
		Type: "resize",
		Rows: 31,
		Cols: 92,
	}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, conn, inputMessage{
		Type: "input",
		Data: "size\n",
	}); err != nil {
		t.Fatal(err)
	}
	readUntil(t, ctx, conn, "31 92")

	// A second browser can also type and resize the shared terminal.
	second, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.CloseNow() }()
	readUntil(t, ctx, second, "31 92")

	// Both browsers receive the same live output.
	if err := wsjson.Write(ctx, second, inputMessage{
		Type: "input",
		Data: "broadcast\n",
	}); err != nil {
		t.Fatal(err)
	}
	readUntil(t, ctx, conn, "received: broadcast")
	readUntil(t, ctx, second, "received: broadcast")

	if err := wsjson.Write(ctx, second, inputMessage{
		Type: "resize",
		Rows: 40,
		Cols: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, second, inputMessage{
		Type: "input",
		Data: "size\n",
	}); err != nil {
		t.Fatal(err)
	}
	readUntil(t, ctx, conn, "40 100")
	readUntil(t, ctx, second, "40 100")

	// Concurrent messages from different browsers reach the harness as whole writes.
	inputs := []string{"first\n", "second\n"}
	written := make(chan error, 2)
	for i, browser := range []*websocket.Conn{conn, second} {
		go func() {
			written <- wsjson.Write(ctx, browser, inputMessage{
				Type: "input",
				Data: inputs[i],
			})
		}()
	}
	for range 2 {
		if err := <-written; err != nil {
			t.Fatal(err)
		}
	}
	readUntil(t, ctx, conn, "received: first", "received: second")
	readUntil(t, ctx, second, "received: first", "received: second")

	_ = second.CloseNow()
	_ = conn.Close(websocket.StatusNormalClosure, "")

	// Disconnecting leaves the harness and its output history intact.
	reconnected, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reconnected.CloseNow() }()
	readUntil(t, ctx, reconnected, "31 92")
}

func readUntil(t *testing.T, ctx context.Context, conn *websocket.Conn, want ...string) {
	t.Helper()

	var text strings.Builder
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read %q: %v (received %q)", want, err, text.String())
		}
		if kind != websocket.MessageBinary {
			t.Fatalf("expected terminal bytes, got message type %v", kind)
		}
		text.Write(data)

		matched := true
		for _, value := range want {
			matched = matched && strings.Contains(text.String(), value)
		}
		if matched {
			return
		}
	}
}

func TestTerminalBoundsSlowReaders(t *testing.T) {
	terminal := newTerminal()
	slow, _ := terminal.attach()

	chunk := make([]byte, 32<<10)
	for range 40 {
		_, _ = terminal.Write(chunk)
	}

	if len(terminal.history) != replayLimit {
		t.Fatalf("history size = %d", len(terminal.history))
	}
	if terminal.clients[slow] {
		t.Fatal("slow reader retained")
	}

	terminal.detach(slow)
	next, history := terminal.attach()
	if len(history) != replayLimit || !terminal.clients[next] {
		t.Fatal("new attachment did not receive history and subscribe")
	}
	terminal.detach(next)
}

func TestTerminalConnectionCloses(t *testing.T) {
	useFakeCodex(t)

	for _, reason := range []string{"shutdown", "exit", "binary input", "invalid resize"} {
		t.Run(reason, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			server, err := Start(ctx, "127.0.0.1", 0, "codex")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cancel()
				if err := server.Wait(); err != nil {
					t.Error(err)
				}
			})

			address := net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
			url := "ws://" + address + "/pty"
			conn, _, err := websocket.Dial(ctx, url, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.CloseNow() }()
			readUntil(t, ctx, conn, "fake harness ready")

			switch reason {
			case "shutdown":
				if err := server.Shutdown(ctx); err != nil {
					t.Fatal(err)
				}
				if err := server.Wait(); err != nil {
					t.Fatal(err)
				}
			case "exit":
				msg := inputMessage{
					Type: "input",
					Data: "last output\nexit\n",
				}
				if err := wsjson.Write(ctx, conn, msg); err != nil {
					t.Fatal(err)
				}
				// Receive the last output before the connection closes.
				readUntil(t, ctx, conn, "received: last output")
			case "binary input":
				if err := conn.Write(ctx, websocket.MessageBinary, []byte("input")); err != nil {
					t.Fatal(err)
				}
			case "invalid resize":
				if err := wsjson.Write(ctx, conn, inputMessage{Type: "resize"}); err != nil {
					t.Fatal(err)
				}
			}

			// The connection must close before the test times out.
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					break
				}
			}
			if err := ctx.Err(); err != nil {
				t.Fatalf("connection remained open: %v", err)
			}
		})
	}
}
