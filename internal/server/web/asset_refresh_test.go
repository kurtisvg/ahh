package web_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func TestPageLoadsCurrentRouterWithCachedOldModules(t *testing.T) {
	testConnections(t, "", func(t *testing.T, f browserTest) {
		upstream, err := url.Parse(f.baseURL)
		requireNoError(t, err)
		prefix := strings.TrimRight(upstream.Path, "/")
		upstream.Path = ""
		proxy := httputil.NewSingleHostReverseProxy(upstream)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == prefix+"/legacy":
				w.Header().Set("Content-Type", "text/html")
				_, _ = fmt.Fprintf(w, `<html><head><base href="%s/"></head><body><script type="module" src="app.js"></script></body></html>`, prefix)
			case r.URL.RawQuery == "" && r.URL.Path == prefix+"/app.js":
				w.Header().Set("Content-Type", "application/javascript")
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				_, _ = fmt.Fprint(w, `import {renderProjects} from './project-page.js'; renderProjects();`)
			case r.URL.RawQuery == "" && r.URL.Path == prefix+"/project-page.js":
				w.Header().Set("Content-Type", "application/javascript")
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				_, _ = fmt.Fprint(w, `export function renderProjects() { document.body.textContent = 'Page not found'; }`)
			default:
				proxy.ServeHTTP(w, r)
			}
		}))
		defer server.Close()
		_, err = f.page.Goto(server.URL + prefix + "/legacy")
		requireNoError(t, err)
		expect := playwright.NewPlaywrightAssertions()
		requireNoError(t, expect.Locator(f.page.Locator("body")).ToHaveText("Page not found"))
		// The new page must bypass both the old entry module and its cached imports.
		_, err = f.page.Goto(server.URL + prefix + "/p/fixture")
		requireNoError(t, err)
		requireNoError(t, expect.Locator(f.page.Locator("#project-name")).ToHaveValue("Fixture"))
	})
}
