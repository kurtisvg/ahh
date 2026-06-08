package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

	if got := rec.Body.String(); !strings.Contains(got, "Local server setup") {
		t.Fatalf("body does not contain shell marker: %q", got)
	}

	if got := rec.Body.String(); !strings.Contains(got, "<style>") || !strings.Contains(got, ".app-shell") {
		t.Fatalf("body does not contain inline shell styles: %q", got)
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
