package wrapper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestHeartbeatDropsUnresponsiveConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	wrapper := &Server{terminal: newTerminal()}
	closed := make(chan string, 2)
	handler := wrapper.servePTY(ctx)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, r)
		closed <- r.URL.Query().Get("client")
	}))
	defer server.Close()

	var responsive context.Context
	for _, name := range []string{"responsive", "silent"} {
		url := "ws" + strings.TrimPrefix(server.URL, "http") + "?client=" + name
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.CloseNow() }()

		var replay replayMessage
		if err := wsjson.Read(ctx, conn, &replay); err != nil {
			t.Fatal(err)
		}
		if name == "responsive" {
			// Keep handling pings on this connection; leave the other unread.
			responsive = conn.CloseRead(ctx)
		}
	}

	select {
	case name := <-closed:
		if name != "silent" || responsive.Err() != nil {
			t.Fatal("heartbeat disconnected the responsive client")
		}
	case <-ctx.Done():
		t.Fatal("heartbeat did not release the silent client")
	}
}
