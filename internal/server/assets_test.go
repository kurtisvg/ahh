package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPageBase(t *testing.T) {
	for _, base := range []string{"/", "/proxy/40443/"} {
		page, err := servePage(base)
		if err != nil {
			t.Fatal(err)
		}
		for _, route := range []string{"/", "/terminal", "/p/id", "/p/id/c"} {
			t.Run(base+route, func(t *testing.T) {
				response := httptest.NewRecorder()
				page.ServeHTTP(response, httptest.NewRequest("GET", route, nil))
				if !strings.Contains(response.Body.String(), `<base href="`+base+`">`) {
					t.Fatal(response.Body.String())
				}
			})
		}
	}
}

func TestAssetsRevalidate(t *testing.T) {
	assets := serveAssets()
	for _, prefix := range []string{"", "/proxy/40441"} {
		for _, path := range []string{"/app.js", "/app.css", "/project-actions.js", "/vendor/xterm.js", "/vendor/xterm.css"} {
			t.Run(prefix+path, func(t *testing.T) {
				response := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, prefix+path, nil)
				handler := http.StripPrefix(prefix, assets)
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusOK || response.Body.Len() == 0 {
					t.Fatalf("asset response: status=%d, bytes=%d", response.Code, response.Body.Len())
				}
				if got := response.Header().Get("Cache-Control"); got != "no-cache" {
					t.Fatalf("Cache-Control = %q, want no-cache", got)
				}
				etag := response.Header().Get("ETag")
				if etag == "" {
					t.Fatal("asset has no ETag")
				}
				request.Header.Set("If-None-Match", etag)
				cached := httptest.NewRecorder()
				handler.ServeHTTP(cached, request)
				if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 ||
					cached.Header().Get("ETag") != etag ||
					cached.Header().Get("Cache-Control") != "no-cache" {
					t.Fatalf("revalidation: status=%d, bytes=%d, headers=%v", cached.Code, cached.Body.Len(), cached.Header())
				}
			})
		}
	}
}

func TestAssetETagChangesWithContent(t *testing.T) {
	var etag string
	for _, content := range []string{"old build", "new build"} {
		files := fstest.MapFS{"app.js": {Data: []byte(content)}}
		request := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		request.Header.Set("If-None-Match", etag)
		response := httptest.NewRecorder()
		serveAssetFiles(files).ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != content {
			t.Fatalf("new content: status=%d, body=%q", response.Code, response.Body.String())
		}
		current := response.Header().Get("ETag")
		if current == "" || current == etag {
			t.Fatalf("ETag did not change: %q", current)
		}
		etag = current
	}
}
