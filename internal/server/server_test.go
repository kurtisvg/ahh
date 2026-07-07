package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kurtisvg/ahh/internal/server/sessions"
	"github.com/kurtisvg/ahh/internal/wrapper"
)

func TestServerHTTP(t *testing.T) {
	tests := []struct {
		name             string
		method           string
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
			name:             "serves app styles",
			path:             "/assets/app.css",
			wantStatus:       http.StatusOK,
			wantBodyContains: ".app-shell",
		},
		{
			name:             "serves app script",
			path:             "/assets/app.js",
			wantStatus:       http.StatusOK,
			wantBodyContains: "terminalSocketURL",
		},
		{
			name:             "reports server readiness",
			path:             "/ready",
			wantStatus:       http.StatusOK,
			wantBodyContains: "ready",
		},
		{
			name:       "reports missing page",
			path:       "/missing",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "reports missing api endpoint",
			path:       "/api/missing",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "rejects wrong session tty method",
			method:     http.MethodPost,
			path:       "/api/sessions/not-a-session/tty",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := startTestServer(t, nil)
			defer shutdownTestServer(t, server)

			client := &http.Client{
				Timeout: 2 * time.Second,
			}
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}

			req, err := http.NewRequestWithContext(
				t.Context(),
				method,
				"http://"+server.Addr+tt.path,
				nil,
			)
			if err != nil {
				t.Fatalf("build %s %s request: %v", method, tt.path, err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", method, tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("%s %s status = %d, want %d", method, tt.path, resp.StatusCode, tt.wantStatus)
			}
			if tt.wantBodyContains == "" {
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read response body: %v", err)
			}
			if !strings.Contains(string(body), tt.wantBodyContains) {
				t.Fatalf("%s %s body = %q, want containing %q", method, tt.path, body, tt.wantBodyContains)
			}
		})
	}
}

func TestStartRejectsNilSessionManager(t *testing.T) {
	server, err := Start(t.Context(), "127.0.0.1:0", WithSessionManager(nil))
	if err == nil {
		if server != nil {
			shutdownTestServer(t, server)
		}
		t.Fatal("Start() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "session manager is required") {
		t.Fatalf("Start() error = %q, want containing session manager is required", err.Error())
	}
}

func TestTerminalPageUsesProxySafePaths(t *testing.T) {
	page := string(readAsset(t, "assets/index.html"))
	app := string(readAsset(t, "assets/app.js"))
	wants := []string{
		`href="assets/xterm.css"`,
		`href="assets/app.css"`,
		`src="assets/xterm.js"`,
		`src="assets/addon-fit.js"`,
		`src="assets/app.js"`,
	}
	for _, want := range wants {
		if !strings.Contains(page, want) {
			t.Fatalf("terminal page does not contain %q", want)
		}
	}

	if want := `return new URL(path, window.location.origin + basePath)`; !strings.Contains(app, want) {
		t.Fatalf("app script does not contain %q", want)
	}

	for _, bad := range []string{`href="/assets/`, `src="/assets/`, `host + '/pty'`} {
		if strings.Contains(page, bad) || strings.Contains(app, bad) {
			t.Fatalf("terminal page contains proxy-unsafe path %q", bad)
		}
	}

	for _, bad := range []string{`<style>`, `<script>`} {
		if strings.Contains(page, bad) {
			t.Fatalf("terminal page still contains inline asset tag %q", bad)
		}
	}
}

func TestAppScriptIncludesConnectionLifecycleStates(t *testing.T) {
	app := string(readAsset(t, "assets/app.js"))
	wants := []string{
		"scheduleReconnect",
		"readReadyState",
		"startReadinessPolling",
		"harness-exited",
	}
	for _, want := range wants {
		if !strings.Contains(app, want) {
			t.Fatalf("app script does not contain %q", want)
		}
	}
}

func TestServerTTYWebSocketProxy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	input := make(chan string, 1)
	fake := newFakeWrapperServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	manager := newTestSessionManager(t, func(context.Context) (wrapper.Wrapper, error) {
		return fake, nil
	})
	session, err := manager.Create(ctx, "terminal")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	server := startTestServer(t, manager)
	defer shutdownTestServer(t, server)

	conn, _, err := websocket.Dial(ctx, "ws://"+server.Addr+"/api/sessions/"+session.ID()+"/tty", nil)
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

type fakeWrapperServer struct {
	server    *httptest.Server
	done      chan struct{}
	closeOnce sync.Once
}

func newFakeWrapperServer(handler http.Handler) *fakeWrapperServer {
	return &fakeWrapperServer{
		server: httptest.NewServer(handler),
		done:   make(chan struct{}),
	}
}

func (s *fakeWrapperServer) Address() string {
	return strings.TrimPrefix(s.server.URL, "http://")
}

func (s *fakeWrapperServer) Wait() error {
	<-s.done

	return nil
}

func (s *fakeWrapperServer) Shutdown(context.Context) error {
	s.closeOnce.Do(func() {
		s.server.Close()
		close(s.done)
	})

	return nil
}

func startTestServer(t *testing.T, manager *sessions.Manager) *Server {
	t.Helper()

	if manager == nil {
		manager = newTestSessionManager(t, func(context.Context) (wrapper.Wrapper, error) {
			return nil, fmt.Errorf("unexpected session start")
		})
	}

	server, err := Start(t.Context(), "127.0.0.1:0", WithSessionManager(manager))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	return server
}

func newTestSessionManager(
	t *testing.T,
	startWrapper func(context.Context) (wrapper.Wrapper, error),
) *sessions.Manager {
	t.Helper()

	manager, err := sessions.New(sessions.WithStartWrapper(startWrapper))
	if err != nil {
		t.Fatalf("sessions.New() error = %v", err)
	}

	return manager
}

func shutdownTestServer(t *testing.T, server *Server) {
	t.Helper()

	if err := server.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := server.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func readAsset(t *testing.T, name string) []byte {
	t.Helper()

	data, err := assetsFS.ReadFile(name)
	if err != nil {
		t.Fatalf("read embedded asset %q: %v", name, err)
	}

	return data
}
