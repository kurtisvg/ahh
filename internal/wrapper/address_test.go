package wrapper

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestStandaloneAddresses(t *testing.T) {
	useFakeCodex(t)

	for _, address := range []string{"127.0.0.1", "localhost", "0.0.0.0"} {
		t.Run(address, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			server, err := Start(ctx, address, 0, "codex")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cancel()
				if err := server.Wait(); err != nil {
					t.Error(err)
				}
			})
			port := strconv.Itoa(server.Port)
			host := server.Address
			if net.ParseIP(host).IsUnspecified() {
				host = "127.0.0.1"
			}
			for _, host := range []string{host, "localhost"} {
				url := "http://" + net.JoinHostPort(host, port)
				client := &http.Client{Timeout: time.Second}
				response, err := client.Get(url)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != http.StatusOK {
					t.Fatalf("%s returned %d", url, response.StatusCode)
				}
				for _, origin := range []string{url, "https://foreign.example"} {
					conn, response, err := websocket.Dial(ctx, "ws://"+net.JoinHostPort(host, port)+"/pty", &websocket.DialOptions{
						HTTPHeader: http.Header{"Origin": {origin}},
					})
					if conn != nil {
						_ = conn.CloseNow()
					}
					if origin == url && err != nil {
						t.Fatal(err)
					}
					if origin != url && (err == nil || response == nil || response.StatusCode != http.StatusForbidden) {
						t.Fatal("cross-origin terminal request accepted")
					}
				}
			}
		})
	}
}
