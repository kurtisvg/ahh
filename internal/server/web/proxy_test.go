package web_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
)

const browserProxyPath = "/proxy/8082/"

// Model code-server: require a path prefix, strip it, and preserve Host/Origin.
func startBrowserProxy(t *testing.T, target string) string {
	t.Helper()
	upstream, err := url.Parse(target)
	requireNoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	prefix := strings.TrimSuffix(browserProxyPath, "/")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, prefix+"/") {
			http.NotFound(w, r)
			return
		}
		r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		proxy.ServeHTTP(w, r)
	}))

	// HTTP shutdown excludes hijacked WebSockets; retain and close those too.
	var mu sync.Mutex
	connections := make(map[net.Conn]struct{})
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		mu.Lock()
		defer mu.Unlock()
		switch state {
		case http.StateNew:
			connections[conn] = struct{}{}
		case http.StateClosed:
			delete(connections, conn)
		}
	}
	server.Start()
	t.Cleanup(func() {
		mu.Lock()
		for conn := range connections {
			_ = conn.Close()
		}
		mu.Unlock()
		server.Close()
	})
	return server.URL + prefix + "/"
}
