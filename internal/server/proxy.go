package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httputil"
)

// proxyPTY forwards terminal connections and closes them when requests are canceled.
func (s *Server) proxyPTY(ctx context.Context, protection *http.CrossOriginProtection) http.HandlerFunc {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(s.wrapper.url)
			// Origin is checked at the public boundary, before proxying to loopback.
			r.Out.Header.Del("Origin")
		},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		s.requestsMu.Lock()
		if ctx.Err() != nil {
			s.requestsMu.Unlock()
			http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
			return
		}
		s.requests.Add(1)
		s.requestsMu.Unlock()
		defer s.requests.Done()

		// WebSocket upgrades use GET but allow terminal input, so check as a write.
		check := *r
		check.Method = http.MethodPost
		if err := protection.Check(&check); err != nil {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}

		// HTTP shutdown does not close hijacked connections. Close the frontend
		// on cancellation so a client that stops reading cannot hold up the proxy.
		var stop func() bool
		defer func() {
			if stop != nil {
				stop()
			}
		}()
		writer := &proxyWriter{
			ResponseWriter: w,
			onHijack: func(conn net.Conn) {
				stop = context.AfterFunc(r.Context(), func() { _ = conn.Close() })
			},
		}
		proxy.ServeHTTP(writer, r)
	}
}

// proxyWriter reports the frontend connection when the proxy upgrades it.
type proxyWriter struct {
	http.ResponseWriter
	onHijack func(net.Conn)
}

// Hijack takes over the connection and lets the caller arrange to close it on cancellation.
func (w *proxyWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, buffer, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.onHijack(conn)
	}
	return conn, buffer, err
}

func (w *proxyWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
