package wrapper

import (
	"context"
	"encoding/binary"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestAcknowledgmentsAfterOutputBecomesIdle(t *testing.T) {
	terminal := newTerminal()
	wrapper := &Server{terminal: terminal}
	server := httptest.NewServer(wrapper.servePTY(t.Context()))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()

	var replay replayMessage
	if err := wsjson.Read(ctx, conn, &replay); err != nil {
		t.Fatal(err)
	}

	// Render a burst only after output becomes idle, exceeding the acknowledgment queue.
	for i := range 32 {
		_, _ = terminal.Write([]byte("x"))
		kind, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if kind != websocket.MessageBinary || len(data) != 9 || binary.BigEndian.Uint64(data) != uint64(i) {
			t.Fatalf("unexpected frame: %v %x", kind, data)
		}
	}

	for i := range 32 {
		if err := wsjson.Write(ctx, conn, inputMessage{Type: "ack", Offset: uint64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}

	// Ping follows the acknowledgments; new output starts once the reader accepts them.
	pingDone := make(chan error, 1)
	go func() {
		err := conn.Ping(ctx)
		if err == nil {
			_, _ = terminal.Write([]byte("healthy"))
		}
		pingDone <- err
	}()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 || string(data[8:]) != "healthy" {
		t.Fatalf("output = %q", data)
	}
	if err := <-pingDone; err != nil {
		t.Fatal(err)
	}
}

func TestRejectInvalidIdleAcknowledgments(t *testing.T) {
	for name, offsets := range map[string][]uint64{"ahead": {2}, "backward": {1, 0}} {
		t.Run(name, func(t *testing.T) {
			terminal := newTerminal()
			_, _ = terminal.Write([]byte("x"))
			wrapper := &Server{terminal: terminal}
			server := httptest.NewServer(wrapper.servePTY(t.Context()))
			defer server.Close()

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.CloseNow() }()

			var replay replayMessage
			if err := wsjson.Read(ctx, conn, &replay); err != nil {
				t.Fatal(err)
			}
			if _, _, err := conn.Read(ctx); err != nil {
				t.Fatal(err)
			}

			for _, offset := range offsets {
				if err := wsjson.Write(ctx, conn, inputMessage{Type: "ack", Offset: offset}); err != nil {
					t.Fatal(err)
				}
			}

			_, _, err = conn.Read(ctx)
			if err == nil || ctx.Err() != nil {
				t.Fatalf("invalid ACKs %v were not rejected promptly: %v", offsets, err)
			}
		})
	}
}
