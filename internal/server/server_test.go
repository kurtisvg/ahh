package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestServerHTTP(t *testing.T) {
	wrapper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ready from wrapper\n"))
	}))
	defer wrapper.Close()

	tests := []struct {
		name             string
		path             string
		wantStatus       int
		wantBodyContains string
	}{
		{
			name:             "serves terminal page",
			path:             "/",
			wantStatus:       http.StatusOK,
			wantBodyContains: "Claude Code PTY",
		},
		{
			name:             "serves terminal assets",
			path:             "/assets/xterm.css",
			wantStatus:       http.StatusOK,
			wantBodyContains: ".xterm",
		},
		{
			name:             "proxies wrapper readiness",
			path:             "/ready",
			wantStatus:       http.StatusOK,
			wantBodyContains: "ready from wrapper",
		},
		{
			name:       "reports missing page",
			path:       "/missing",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := startTestServer(t, wrapper.URL)
			defer closeTestServer(t, server)

			client := &http.Client{
				Timeout: 2 * time.Second,
			}
			resp, err := client.Get(server.URL() + tt.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("GET %s status = %d, want %d", tt.path, resp.StatusCode, tt.wantStatus)
			}
			if tt.wantBodyContains == "" {
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read response body: %v", err)
			}
			if !strings.Contains(string(body), tt.wantBodyContains) {
				t.Fatalf("GET %s body = %q, want containing %q", tt.path, body, tt.wantBodyContains)
			}
		})
	}
}

func TestTerminalPageUsesProxySafePaths(t *testing.T) {
	page := string(readAsset(t, "assets/index.html"))
	wants := []string{
		`href="assets/xterm.css"`,
		`src="assets/xterm.js"`,
		`src="assets/addon-fit.js"`,
		`new URL('pty', window.location.origin + basePath)`,
	}
	for _, want := range wants {
		if !strings.Contains(page, want) {
			t.Fatalf("terminal page does not contain %q", want)
		}
	}

	for _, bad := range []string{`href="/assets/`, `src="/assets/`, `host + '/pty'`} {
		if strings.Contains(page, bad) {
			t.Fatalf("terminal page contains proxy-unsafe path %q", bad)
		}
	}
}

func TestServerPTYWebSocketProxy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	input := make(chan string, 1)
	wrapper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pty" {
			http.NotFound(w, r)
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		if err := conn.Write(r.Context(), websocket.MessageBinary, []byte("hello\r\n")); err != nil {
			return
		}

		messageType, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if messageType == websocket.MessageText {
			input <- string(data)
		}
	}))
	defer wrapper.Close()

	server := startTestServer(t, wrapper.URL)
	defer closeTestServer(t, server)

	conn, _, err := websocket.Dial(ctx, websocketURL(server.URL(), "/pty"), nil)
	if err != nil {
		t.Fatalf("dial server websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read server websocket: %v", err)
	}
	if messageType != websocket.MessageBinary {
		t.Fatalf("websocket message type = %v, want %v", messageType, websocket.MessageBinary)
	}
	if string(data) != "hello\r\n" {
		t.Fatalf("websocket data = %q, want %q", data, "hello\r\n")
	}

	const terminalInput = `{"type":"input","data":"pwd\r"}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(terminalInput)); err != nil {
		t.Fatalf("write terminal input: %v", err)
	}
	select {
	case got := <-input:
		if got != terminalInput {
			t.Fatalf("proxied terminal input = %q, want %q", got, terminalInput)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func startTestServer(t *testing.T, wrapperServer string) *Server {
	t.Helper()

	server, err := startWithWrapperServer(t.Context(), wrapperServer, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	return server
}

func closeTestServer(t *testing.T, server *Server) {
	t.Helper()

	if err := server.Close(t.Context()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := server.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func websocketURL(serverURL, path string) string {
	return "ws://" + strings.TrimPrefix(serverURL, "http://") + path
}

func readAsset(t *testing.T, name string) []byte {
	t.Helper()

	data, err := assetsFS.ReadFile(name)
	if err != nil {
		t.Fatalf("read embedded asset %q: %v", name, err)
	}

	return data
}
