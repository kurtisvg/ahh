package wrapper

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestOutputCompletionMarker(t *testing.T) {
	for _, cursor := range []string{"", "?cursor=3", "?cursor=5"} {
		t.Run(cursor, func(t *testing.T) {
			terminal := newTerminal()
			_, _ = terminal.Write([]byte("final"))
			terminal.finish()
			wrapper := &Server{terminal: terminal}
			server := httptest.NewServer(wrapper.servePTY(t.Context()))
			defer server.Close()

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+cursor, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.CloseNow() }()

			var replay replayMessage
			if err := wsjson.Read(ctx, conn, &replay); err != nil {
				t.Fatal(err)
			}
			if replay.Type != "replay" {
				t.Fatalf("initial message = %+v", replay)
			}

			if cursor != "?cursor=5" {
				_, data, err := conn.Read(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(data) < 8 {
					t.Fatalf("short frame: %x", data)
				}
				offset := binary.BigEndian.Uint64(data)
				if offset > 5 || string(data[8:]) != "final"[offset:] {
					t.Fatalf("replay = %x", data)
				}
			}

			var end endMessage
			if err := wsjson.Read(ctx, conn, &end); err != nil {
				t.Fatal(err)
			}
			if end.Type != "end" || end.Offset != 5 {
				t.Fatalf("completion = %+v", end)
			}
		})
	}
}

func TestDroppedOutputDoesNotCompleteReplay(t *testing.T) {
	terminal := newTerminal()
	state, err := terminal.attach(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.detach(state.client)

	// Overflow this reader before starting delivery, then exit the process.
	// Its closed queue must not be mistaken for all final output being delivered.
	for range 17 {
		_, _ = terminal.Write([]byte("x"))
	}
	terminal.finish()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		if err := sendOutput(r.Context(), conn, terminal, state.client, state.history); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()

	for range 16 {
		kind, _, err := conn.Read(ctx)
		if err != nil || kind != websocket.MessageBinary {
			t.Fatalf("queued output: %v, %v", kind, err)
		}
	}
	if _, data, err := conn.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatalf("dropped reader must disconnect without completion: %s, %v", data, err)
	}
}
