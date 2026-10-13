package wrapper

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestUnacknowledgedOutputIsBounded(t *testing.T) {
	terminal := newTerminal()
	_, _ = terminal.Write(bytes.Repeat([]byte("x"), replayLimit))
	state, err := terminal.attach(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.detach(state.client)

	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		done <- sendOutput(r.Context(), conn, terminal, state.client, state.history)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(64 * 1024)

	received := 0
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		received += len(data) - 8
	}
	if received != 128*1024 {
		t.Fatalf("unacknowledged bytes = %d", received)
	}
	if err := <-done; err == nil || err.Error() != "terminal reader stalled" {
		t.Fatalf("slow reader result = %v", err)
	}
}
