package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
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
			wantBodyContains: "New conversation",
		},
		{
			name:             "serves bookmarked conversation",
			path:             "/conversations/bookmarked-id",
			wantStatus:       http.StatusOK,
			wantBodyContains: "New conversation",
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
			name:             "serves assets from bookmarked conversation",
			path:             "/conversations/assets/app.js",
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
			server := startTestServer(t)
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

func TestStartRejectsEmptyStateDir(t *testing.T) {
	_, err := Start(t.Context(), "127.0.0.1:0", WithStateDir(" \t "))
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "state directory is required") {
		t.Fatalf("Start() error = %q, want state directory is required", err.Error())
	}
}

func TestServerSessionsAPI(t *testing.T) {
	factory := &fakeWrapperFactory{}
	base := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		base,
		base.Add(time.Minute),
	}
	manager, err := sessions.NewManager(
		t.Context(),
		sessions.WithStartWrapper(factory.start),
		sessions.WithClock(func() time.Time {
			if len(times) == 0 {
				return base.Add(2 * time.Minute)
			}

			next := times[0]
			times = times[1:]
			return next
		}),
	)
	if err != nil {
		t.Fatalf("sessions.NewManager() error = %v", err)
	}

	server := startTestServer(t, WithSessionManager(manager))
	defer shutdownTestServer(t, server)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get("http://" + server.Addr + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var initial sessionsResponse
	decodeJSON(t, resp, &initial)
	if len(initial.Sessions) != 0 {
		t.Fatalf("initial sessions = %d, want 0", len(initial.Sessions))
	}

	resp, err = client.Post("http://"+server.Addr+"/api/sessions", "application/json", strings.NewReader(`{"name":"   "}`))
	if err != nil {
		t.Fatalf("POST blank session: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
	var apiErr errorResponse
	decodeJSON(t, resp, &apiErr)
	if apiErr.Error != "session name is required" {
		t.Fatalf("blank session error = %q, want session name is required", apiErr.Error)
	}

	first := createSessionViaAPI(t, client, server, "First")
	second := createSessionViaAPI(t, client, server, "Second")
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for _, session := range []sessions.Metadata{first, second} {
		if !uuidPattern.MatchString(session.ID) {
			t.Fatalf("session id %q is not a UUID v4", session.ID)
		}
		if session.Status != sessions.StatusRunning {
			t.Fatalf("session %q status = %q, want %q", session.Name, session.Status, sessions.StatusRunning)
		}
	}

	resp, err = client.Get("http://" + server.Addr + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions after create: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var listed sessionsResponse
	decodeJSON(t, resp, &listed)
	if len(listed.Sessions) != 2 {
		t.Fatalf("listed sessions = %d, want 2", len(listed.Sessions))
	}
	if listed.Sessions[0].ID != second.ID || listed.Sessions[1].ID != first.ID {
		t.Fatalf("session order = [%q, %q], want newest-first [%q, %q]",
			listed.Sessions[0].Name,
			listed.Sessions[1].Name,
			second.Name,
			first.Name,
		)
	}
}

func TestServerDeleteSessionAPI(t *testing.T) {
	factory := &fakeWrapperFactory{}
	manager := newTestSessionManager(t, factory.start)
	server := startTestServer(t, WithSessionManager(manager))
	defer shutdownTestServer(t, server)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	session := createSessionViaAPI(t, client, server, "delete me")
	if got := factory.wrapperCount(); got != 1 {
		t.Fatalf("started wrappers = %d, want 1", got)
	}

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodDelete,
		"http://"+server.Addr+"/api/sessions/"+session.ID,
		nil,
	)
	if err != nil {
		t.Fatalf("new delete request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/sessions/%s: %v", session.ID, err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)

	select {
	case <-factory.wrapper(0).done:
	case <-time.After(2 * time.Second):
		t.Fatal("deleted session wrapper was not shut down")
	}

	resp, err = client.Get("http://" + server.Addr + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions after delete: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var listed sessionsResponse
	decodeJSON(t, resp, &listed)
	if len(listed.Sessions) != 0 {
		t.Fatalf("listed sessions after delete = %d, want 0", len(listed.Sessions))
	}

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("second DELETE /api/sessions/%s: %v", session.ID, err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

func TestServerPersistsSessionMetadata(t *testing.T) {
	stateDir := t.TempDir()
	factory := &fakeWrapperFactory{}
	manager, err := sessions.NewManager(
		t.Context(),
		sessions.WithStartWrapper(factory.start),
		sessions.WithStateDir(stateDir),
	)
	if err != nil {
		t.Fatalf("sessions.NewManager() error = %v", err)
	}

	server := startTestServer(t, WithSessionManager(manager))
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	session := createSessionViaAPI(t, client, server, "persisted")
	initialStart := factory.startRequest(0)
	if initialStart.SessionID != "" || initialStart.Resume {
		t.Fatalf("initial wrapper start = %+v, want generated session", initialStart)
	}
	if got := factory.wrapper(0).SessionID(); got != session.ID {
		t.Fatalf("created session id = %q, want wrapper session id %q", session.ID, got)
	}
	createdSession, ok := manager.Get(session.ID)
	if !ok {
		t.Fatalf("created session %q not found", session.ID)
	}
	if err := createdSession.MarkResumable(); err != nil {
		t.Fatalf("MarkResumable() error = %v", err)
	}
	shutdownTestServer(t, server)

	if _, err := os.Stat(filepath.Join(stateDir, "sessions", session.ID+".json")); err != nil {
		t.Fatalf("stat persisted session metadata: %v", err)
	}

	restartedFactory := &fakeWrapperFactory{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/pty" {
				http.NotFound(w, r)
				return
			}

			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			_, _, _ = conn.Read(r.Context())
		}),
	}
	restartedManager, err := sessions.NewManager(
		t.Context(),
		sessions.WithStartWrapper(restartedFactory.start),
		sessions.WithStateDir(stateDir),
	)
	if err != nil {
		t.Fatalf("reload sessions.NewManager() error = %v", err)
	}
	restartedServer := startTestServer(t, WithSessionManager(restartedManager))
	defer shutdownTestServer(t, restartedServer)

	resp, err := client.Get("http://" + restartedServer.Addr + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions after restart: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var listed sessionsResponse
	decodeJSON(t, resp, &listed)
	if len(listed.Sessions) != 1 {
		t.Fatalf("restarted sessions = %d, want 1", len(listed.Sessions))
	}
	if listed.Sessions[0].ID != session.ID || listed.Sessions[0].Name != "persisted" {
		t.Fatalf("restarted session = %+v, want id %q name persisted", listed.Sessions[0], session.ID)
	}
	if listed.Sessions[0].Status != sessions.StatusExited {
		t.Fatalf("restarted session status = %q, want %q", listed.Sessions[0].Status, sessions.StatusExited)
	}
	if got := restartedFactory.wrapperCount(); got != 0 {
		t.Fatalf("wrappers started on metadata reload = %d, want 0", got)
	}

	conn, _, err := websocket.Dial(
		t.Context(),
		"ws://"+restartedServer.Addr+"/api/sessions/"+session.ID+"/tty",
		nil,
	)
	if err != nil {
		t.Fatalf("dial persisted session tty: %v", err)
	}
	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close persisted session tty: %v", err)
	}
	waitForSessionStatus(t, restartedManager, session.ID, sessions.StatusRunning)
	if got := restartedFactory.wrapperCount(); got != 1 {
		t.Fatalf("wrappers started after persisted tty connection = %d, want 1", got)
	}
	restoredStart := restartedFactory.startRequest(0)
	if restoredStart.SessionID != session.ID || !restoredStart.Resume {
		t.Fatalf("restored wrapper start = %+v, want id %q with resume", restoredStart, session.ID)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, "http://"+restartedServer.Addr+"/api/sessions/"+session.ID, nil)
	if err != nil {
		t.Fatalf("new delete request: %v", err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("DELETE persisted session: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)
	if _, err := os.Stat(filepath.Join(stateDir, "sessions", session.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat deleted session metadata error = %v, want not exist", err)
	}
}

func TestServerTTYMissingSession(t *testing.T) {
	factory := &fakeWrapperFactory{}
	manager := newTestSessionManager(t, factory.start)

	server := startTestServer(t, WithSessionManager(manager))
	defer shutdownTestServer(t, server)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	resp, err := client.Get("http://" + server.Addr + "/api/sessions/missing/tty")
	if err != nil {
		t.Fatalf("GET missing tty: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

func TestSubmittedTerminalInput(t *testing.T) {
	tests := []struct {
		name        string
		messageType websocket.MessageType
		data        string
		want        bool
	}{
		{
			name:        "submitted input",
			messageType: websocket.MessageText,
			data:        `{"type":"input","data":"prompt\r"}`,
			want:        true,
		},
		{
			name:        "typing without submission",
			messageType: websocket.MessageText,
			data:        `{"type":"input","data":"prompt"}`,
		},
		{
			name:        "terminal resize",
			messageType: websocket.MessageText,
			data:        `{"type":"resize","rows":24,"cols":80}`,
		},
		{
			name:        "binary message",
			messageType: websocket.MessageBinary,
			data:        `{"type":"input","data":"prompt\r"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := submittedTerminalInput(tt.messageType, []byte(tt.data)); got != tt.want {
				t.Fatalf("submittedTerminalInput() = %t, want %t", got, tt.want)
			}
		})
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

func TestAppScriptUsesConnectionLifecycleStates(t *testing.T) {
	app := string(readAsset(t, "assets/app.js"))
	wants := []string{
		"scheduleReconnect",
		"loadConversations",
		"startConversationPolling",
		"conversationIdFromPath",
		"updateConversationStatuses",
		"setStatus('connected')",
		"setStatus('reconnecting')",
		"setStatus('disconnected')",
	}
	for _, want := range wants {
		if !strings.Contains(app, want) {
			t.Fatalf("app script does not contain %q", want)
		}
	}
	for _, unwanted := range []string{"conversation-exited", "conversation.status"} {
		if strings.Contains(app, unwanted) {
			t.Fatalf("app script contains backend lifecycle state %q", unwanted)
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

	manager := newTestSessionManager(t, func(context.Context, sessions.WrapperStart) (wrapper.Wrapper, error) {
		return fake, nil
	})
	session, err := manager.Create(ctx, "terminal")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	server := startTestServer(t, WithSessionManager(manager))
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
	sessionID string
	server    *httptest.Server
	done      chan struct{}
	closeOnce sync.Once
}

type fakeWrapperFactory struct {
	mu       sync.Mutex
	wrappers []*fakeWrapperServer
	starts   []sessions.WrapperStart
	handler  http.Handler
}

func (f *fakeWrapperFactory) start(
	_ context.Context,
	start sessions.WrapperStart,
) (wrapper.Wrapper, error) {
	handler := f.handler
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	fake := newFakeWrapperServer(handler)
	if start.SessionID != "" {
		fake.sessionID = start.SessionID
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.wrappers = append(f.wrappers, fake)
	f.starts = append(f.starts, start)
	return fake, nil
}

func (f *fakeWrapperFactory) wrapperCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.wrappers)
}

func (f *fakeWrapperFactory) wrapper(index int) *fakeWrapperServer {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.wrappers[index]
}

func (f *fakeWrapperFactory) startRequest(index int) sessions.WrapperStart {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.starts[index]
}

func newFakeWrapperServer(handler http.Handler) *fakeWrapperServer {
	return &fakeWrapperServer{
		sessionID: uuid.NewString(),
		server:    httptest.NewServer(handler),
		done:      make(chan struct{}),
	}
}

func (s *fakeWrapperServer) Address() string {
	return strings.TrimPrefix(s.server.URL, "http://")
}

func (s *fakeWrapperServer) SessionID() string {
	return s.sessionID
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

func startTestServer(t *testing.T, opts ...Option) *Server {
	t.Helper()

	server, err := Start(t.Context(), "127.0.0.1:0", opts...)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	return server
}

func newTestSessionManager(
	t *testing.T,
	startWrapper func(context.Context, sessions.WrapperStart) (wrapper.Wrapper, error),
) *sessions.Manager {
	t.Helper()

	manager, err := sessions.NewManager(t.Context(), sessions.WithStartWrapper(startWrapper))
	if err != nil {
		t.Fatalf("sessions.NewManager() error = %v", err)
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

func createSessionViaAPI(t *testing.T, client *http.Client, server *Server, name string) sessions.Metadata {
	t.Helper()

	requestBody := bytes.NewBufferString(fmt.Sprintf(`{"name":%q}`, name))
	resp, err := client.Post("http://"+server.Addr+"/api/sessions", "application/json", requestBody)
	if err != nil {
		t.Fatalf("POST /api/sessions: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var session sessions.Metadata
	decodeJSON(t, resp, &session)
	if session.Name != name {
		t.Fatalf("created session name = %q, want %q", session.Name, name)
	}

	return session
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()

	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d, body = %q", resp.StatusCode, want, body)
	}
}

func decodeJSON(t *testing.T, resp *http.Response, value any) {
	t.Helper()

	if err := json.NewDecoder(resp.Body).Decode(value); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}

func waitForSessionStatus(t *testing.T, manager *sessions.Manager, id string, want sessions.Status) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, session := range manager.List() {
			if session.ID == id && session.Status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("session %q did not reach status %q", id, want)
}
