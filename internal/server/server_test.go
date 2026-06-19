package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kurtisvg/ahh/internal/wrapperproc"
)

func TestHealthz(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	NewHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got, want := rec.Body.String(), "ok\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestIndex(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	NewHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got := rec.Body.String(); !strings.Contains(got, "conversation1") {
		t.Fatalf("body does not contain shell marker: %q", got)
	}

	if got := rec.Body.String(); strings.Contains(got, "<style>") {
		t.Fatalf("body contains inline styles: %q", got)
	}

	if got := rec.Body.String(); !strings.Contains(got, `href="./static/styles.css?v=2"`) {
		t.Fatalf("body does not contain relative stylesheet link: %q", got)
	}

	if got := rec.Body.String(); strings.Contains(got, `href="/`) || strings.Contains(got, `src="/`) {
		t.Fatalf("body contains root-relative asset URL: %q", got)
	}
}

func TestStaticStyles(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/static/styles.css", nil)
	rec := httptest.NewRecorder()

	NewHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if got, want := rec.Header().Get("Cache-Control"), "no-store"; got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
	}

	if got := rec.Body.String(); !strings.Contains(got, ".app-shell") {
		t.Fatalf("body does not contain stylesheet marker: %q", got)
	}
}

func TestNotFound(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	NewHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestWrapperStatus(t *testing.T) {
	t.Parallel()

	supervisor := &fakeWrapperSupervisor{
		status: wrapperproc.Status{
			Harness: "claude-code",
			State:   wrapperproc.StateReady,
			Address: "127.0.0.1:18081",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/wrapper/status", nil)
	rec := httptest.NewRecorder()

	NewHandlerWithOptions(Options{WrapperSupervisor: supervisor}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var status wrapperproc.Status
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Harness != "claude-code" {
		t.Fatalf("harness = %q, want %q", status.Harness, "claude-code")
	}
	if status.State != wrapperproc.StateReady {
		t.Fatalf("state = %q, want %q", status.State, wrapperproc.StateReady)
	}
	if status.Address != "127.0.0.1:18081" {
		t.Fatalf("address = %q, want %q", status.Address, "127.0.0.1:18081")
	}
}

func TestServeShutsDownOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln, err := Listen("127.0.0.1", "0")
	if err != nil {
		t.Fatal(err)
	}

	errc := make(chan error, 1)
	go func() {
		errc <- Serve(ctx, ln)
	}()

	client := &http.Client{Timeout: time.Second}
	url := "http://" + ln.Addr().String() + "/healthz"
	waitForHealthz(t, client, url, errc)

	cancel()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("serve returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down after context cancellation")
	}
}

func TestServeStartsAndStopsWrapperSupervisor(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln, err := Listen("127.0.0.1", "0")
	if err != nil {
		t.Fatal(err)
	}
	supervisor := &fakeWrapperSupervisor{
		status: wrapperproc.Status{
			Harness: "claude-code",
			State:   wrapperproc.StateReady,
			Address: "127.0.0.1:18081",
		},
	}

	errc := make(chan error, 1)
	go func() {
		errc <- ServeWithOptions(ctx, ln, Options{WrapperSupervisor: supervisor})
	}()

	client := &http.Client{Timeout: time.Second}
	url := "http://" + ln.Addr().String() + "/api/wrapper/status"
	waitForHealthz(t, client, url, errc)

	cancel()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("serve returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down after context cancellation")
	}

	if got := supervisor.startCount(); got != 1 {
		t.Fatalf("start count = %d, want 1", got)
	}
	if got := supervisor.stopCount(); got != 1 {
		t.Fatalf("stop count = %d, want 1", got)
	}
}

func waitForHealthz(t *testing.T, client *http.Client, url string, errc <-chan error) {
	t.Helper()

	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case err := <-errc:
			t.Fatalf("serve returned before health check passed: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for health check")
		case <-tick.C:
			resp, err := client.Get(url)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
	}
}

type fakeWrapperSupervisor struct {
	mu       sync.Mutex
	starts   int
	stops    int
	status   wrapperproc.Status
	startErr error
	stopErr  error
}

func (f *fakeWrapperSupervisor) Start(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	return f.startErr
}

func (f *fakeWrapperSupervisor) Stop(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return f.stopErr
}

func (f *fakeWrapperSupervisor) Status() wrapperproc.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeWrapperSupervisor) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts
}

func (f *fakeWrapperSupervisor) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stops
}
