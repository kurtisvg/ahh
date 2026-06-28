package wrapper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type terminalSize struct {
	rows uint16
	cols uint16
}

// fakeHarness lets the wrapper exercise PTY reads, writes, resize requests, and
// shutdown without starting a real harness process in unit tests.
type fakeHarness struct {
	output    chan []byte
	input     chan string
	resize    chan terminalSize
	done      chan struct{}
	closeOnce sync.Once
}

func TestServerHTTP(t *testing.T) {
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
			name:             "reports readiness",
			path:             "/ready",
			wantStatus:       http.StatusOK,
			wantBodyContains: "ready",
		},
		{
			name:             "serves terminal assets",
			path:             "/assets/xterm.css",
			wantStatus:       http.StatusOK,
			wantBodyContains: ".xterm",
		},
		{
			name:       "reports missing page",
			path:       "/missing",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFakeHarness()
			server := startTestServer(t, h)
			defer h.Close()
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

func TestServerPTYWebSocketBridge(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	h := newFakeHarness()
	server := startTestServer(t, h)
	defer h.Close()
	defer closeTestServer(t, server)

	conn, _, err := websocket.Dial(ctx, websocketURL(server.URL(), "/pty"), nil)
	if err != nil {
		t.Fatalf("dial terminal websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if err := h.sendOutput(ctx, "hello\r\n"); err != nil {
		t.Fatalf("send fake output: %v", err)
	}
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read terminal websocket: %v", err)
	}
	if messageType != websocket.MessageBinary {
		t.Fatalf("websocket message type = %v, want %v", messageType, websocket.MessageBinary)
	}
	if string(data) != "hello\r\n" {
		t.Fatalf("websocket data = %q, want %q", data, "hello\r\n")
	}

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"input","data":"pwd\r"}`)); err != nil {
		t.Fatalf("write terminal input: %v", err)
	}
	select {
	case input := <-h.input:
		if input != "pwd\r" {
			t.Fatalf("terminal input = %q, want %q", input, "pwd\r")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","rows":42,"cols":120}`)); err != nil {
		t.Fatalf("write terminal resize: %v", err)
	}
	select {
	case size := <-h.resize:
		if size.rows != 42 || size.cols != 120 {
			t.Fatalf("terminal size = %dx%d, want 42x120", size.rows, size.cols)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestServerPTYWebSocketReplaysOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	h := newFakeHarness()
	server := startTestServer(t, h)
	defer h.Close()
	defer closeTestServer(t, server)

	if err := h.sendOutput(ctx, "ready before browser\r\n"); err != nil {
		t.Fatalf("send fake output: %v", err)
	}
	waitForTerminalHistory(t, server, "ready before browser\r\n")

	conn, _, err := websocket.Dial(ctx, websocketURL(server.URL(), "/pty"), nil)
	if err != nil {
		t.Fatalf("dial terminal websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read terminal websocket: %v", err)
	}
	if messageType != websocket.MessageBinary {
		t.Fatalf("websocket message type = %v, want %v", messageType, websocket.MessageBinary)
	}
	if string(data) != "ready before browser\r\n" {
		t.Fatalf("websocket data = %q, want %q", data, "ready before browser\r\n")
	}
}

func newFakeHarness() *fakeHarness {
	return &fakeHarness{
		output: make(chan []byte, 10),
		input:  make(chan string, 10),
		resize: make(chan terminalSize, 10),
		done:   make(chan struct{}),
	}
}

func (h *fakeHarness) Read(p []byte) (int, error) {
	select {
	case <-h.done:
		return 0, io.EOF
	case data, ok := <-h.output:
		if !ok {
			return 0, io.EOF
		}

		return copy(p, data), nil
	}
}

func (h *fakeHarness) Write(p []byte) (int, error) {
	select {
	case <-h.done:
		return 0, io.ErrClosedPipe
	case h.input <- string(p):
		return len(p), nil
	}
}

func (h *fakeHarness) Resize(rows, cols uint16) error {
	if rows == 0 || cols == 0 {
		return fmt.Errorf("rows and cols must be positive")
	}

	select {
	case <-h.done:
		return io.ErrClosedPipe
	case h.resize <- terminalSize{rows: rows, cols: cols}:
		return nil
	}
}

func (h *fakeHarness) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.done:
		return nil
	}
}

func (h *fakeHarness) Done() bool {
	select {
	case <-h.done:
		return true
	default:
		return false
	}
}

func (h *fakeHarness) Close() {
	h.closeOnce.Do(func() {
		close(h.done)
		close(h.output)
	})
}

func (h *fakeHarness) sendOutput(ctx context.Context, data string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case h.output <- []byte(data):
		return nil
	}
}

func startTestServer(t *testing.T, h *fakeHarness) *Server {
	t.Helper()

	server, err := Start(h, "127.0.0.1:0")
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

func waitForTerminalHistory(t *testing.T, server *Server, want string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		server.terminal.mu.Lock()
		got := string(server.terminal.history)
		server.terminal.mu.Unlock()

		if strings.Contains(got, want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("terminal history does not contain %q", want)
}
