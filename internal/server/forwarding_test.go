package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestForwardedTerminal(t *testing.T) {
	useTestWrapper(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	server, err := start(ctx, "127.0.0.1", 0, os.Args[0], "-test.run=^TestChildWrapper$")
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
	url := "http://" + address
	client := &http.Client{Timeout: time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "localhost:18082"
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("forwarded page returned %d", response.StatusCode)
	}

	for _, test := range []struct {
		name, host, origin, site string
		allowed                  bool
	}{
		{"local forwarded port", "localhost:18082", "http://localhost:18082", "", true},
		{"HTTPS tunnel", "workspace.example", "https://workspace.example", "", true},
		{"proxy rewrites Host", address, "https://workspace.example", "same-origin", true},
		{"foreign origin", "workspace.example", "https://foreign.example", "", false},
		{"cross-site browser", "workspace.example", "https://foreign.example", "cross-site", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := http.Header{"Origin": {test.origin}}
			if test.site != "" {
				headers.Set("Sec-Fetch-Site", test.site)
			}
			conn, response, err := websocket.Dial(ctx, "ws://"+address+"/pty", &websocket.DialOptions{
				Host:       test.host,
				HTTPHeader: headers,
			})
			if !test.allowed {
				if conn != nil {
					_ = conn.CloseNow()
				}
				if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
					t.Fatal("foreign terminal connection was not rejected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.CloseNow() }()

			var replay struct {
				Type string `json:"type"`
			}
			if err := wsjson.Read(ctx, conn, &replay); err != nil || replay.Type != "replay" {
				t.Fatalf("replay message = %+v, error = %v", replay, err)
			}

			kind, _, err := conn.Read(ctx)
			if err != nil || kind != websocket.MessageBinary {
				t.Fatalf("terminal output kind = %v, error = %v", kind, err)
			}
		})
	}
}
