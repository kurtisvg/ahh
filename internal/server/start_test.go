package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/kurtisvg/ahh/internal/store"
)

func startWithStore(
	t *testing.T,
	ctx context.Context,
	config Config,
	storage store.Store,
	executable string,
	args ...string,
) (*Server, error) {
	t.Helper()
	if config.Workspace == "" {
		config.Workspace = t.TempDir()
	}
	lock, err := lockWorkspace(config.Workspace)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	return start(ctx, config, storage, lock, executable, args...)
}

// Existing terminal tests opt into launch; production startup remains lazy.
func startRunning(
	t *testing.T,
	ctx context.Context,
	address string,
	port int,
	executable string,
	args ...string,
) (*Server, error) {
	t.Helper()
	s, err := startWithStore(t, ctx, Config{Address: address, Port: port}, testProjects{}, executable, args...)
	if err != nil {
		return nil, err
	}
	testTerminalRoutes(s)
	response := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/start", nil))
	if response.Code != http.StatusOK {
		_ = s.Shutdown(ctx)
		_ = s.Wait()
		return nil, fmt.Errorf("start terminal: %s", response.Body.String())
	}
	return s, nil
}

func TestStartOnDemand(t *testing.T) {
	useTestWrapper(t)
	s, err := startWithStore(
		t,
		t.Context(),
		Config{Address: "127.0.0.1"},
		testProjects{},
		os.Args[0],
		"-test.run=^TestChildWrapper$",
	)
	if err != nil {
		t.Fatal(err)
	}
	testTerminalRoutes(s)
	t.Cleanup(func() { _ = s.Shutdown(t.Context()); _ = s.Wait() })
	request := func(method, path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		return response
	}
	for range 2 {
		if response := request(http.MethodGet, "/"); response.Code != http.StatusOK {
			t.Fatal(response.Code)
		}
		response := request(http.MethodGet, "/status")
		var state processStatus
		if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || state.State != "stopped" {
			t.Fatalf("initial state: %+v, %v", state, err)
		}
	}
	if response := request(http.MethodGet, "/pty"); response.Code != http.StatusConflict {
		t.Fatal(response.Code)
	}
	var calls sync.WaitGroup
	children := make(chan *wrapperProcess, 4)
	for range 4 {
		calls.Go(func() {
			response := request(http.MethodPost, "/start")
			if response.Code != http.StatusOK {
				t.Errorf("start: %s", response.Body.String())
			}
			children <- s.currentWrapper()
		})
	}
	calls.Wait()
	close(children)
	for child := range children {
		if child == nil || child != s.currentWrapper() {
			t.Fatal("concurrent starts created different wrappers")
		}
	}
}

func TestFailedLaunchKeepsHTTPAvailable(t *testing.T) {
	for _, command := range []string{"/does-not-exist/codex", "sh"} {
		t.Run(command, func(t *testing.T) {
			s, err := startWithStore(t, t.Context(), Config{Address: "127.0.0.1"}, testProjects{}, command, "-c", "echo invalid")
			if err != nil {
				t.Fatal(err)
			}
			testTerminalRoutes(s)
			t.Cleanup(func() { _ = s.Shutdown(t.Context()); _ = s.Wait() })
			for range 2 {
				response := httptest.NewRecorder()
				s.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/start", nil))
				if response.Code != http.StatusServiceUnavailable {
					t.Fatalf("launch: %d", response.Code)
				}
				resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", s.Port))
				if err != nil {
					t.Fatal(err)
				}
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatal("launch failure damaged server state")
				}
				if (s.currentWrapper() != nil) != (command == "sh") {
					t.Fatal("startup failure lost ownership evidence")
				}
			}
		})
	}
}

// currentWrapper returns the process used by transport tests.
func (s *Server) currentWrapper() *wrapperProcess {
	status := s.executions.Status("")
	child, _ := s.executions.Process("", status.Invocation)
	return child
}

func (s *Server) serveTerminal(
	w http.ResponseWriter,
	r *http.Request,
	protection *http.CrossOriginProtection,
) {
	child := s.currentWrapper()
	if child == nil {
		http.Error(w, "terminal has not started", http.StatusConflict)
		return
	}
	select {
	case <-child.done:
		http.Error(w, "invocation has ended", http.StatusGone)
		return
	default:
	}
	proxyPTY(
		w,
		r,
		child.url,
		protection,
	)
}

func (s *Server) serveStart(w http.ResponseWriter, r *http.Request) {
	err := s.executions.startAndWait(r.Context(), "", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	s.serveStatus(w, r)
}

func (s *Server) serveStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.executions.Status(""))
}

// Unscoped routes exist only in tests of the isolated terminal transport.
func testTerminalRoutes(s *Server) {
	mux := http.NewServeMux()
	mux.Handle("/", s.http.Handler)
	mux.HandleFunc("GET /status", s.serveStatus)
	mux.HandleFunc("POST /start", s.serveStart)
	protection := http.NewCrossOriginProtection()
	mux.HandleFunc("GET /pty", func(w http.ResponseWriter, r *http.Request) {
		s.serveTerminal(w, r, protection)
	})
	s.http.Handler = protection.Handler(s.trackRequests(s.http.BaseContext(nil), mux))
}
