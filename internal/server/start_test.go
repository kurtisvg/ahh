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
)

// Existing terminal tests opt into launch; production startup remains lazy.
func startRunning(ctx context.Context, address string, port int, executable string, args ...string) (*Server, error) {
	s, err := start(ctx, address, port, executable, args...)
	if err != nil {
		return nil, err
	}
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
	s, err := start(t.Context(), "127.0.0.1", 0, os.Args[0], "-test.run=^TestChildWrapper$")
	if err != nil {
		t.Fatal(err)
	}
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
			s, err := start(t.Context(), "127.0.0.1", 0, command, "-c", "echo invalid")
			if err != nil {
				t.Fatal(err)
			}
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
				if resp.StatusCode != http.StatusOK || s.currentWrapper() != nil {
					t.Fatal("launch failure damaged server state")
				}
			}
		})
	}
}
