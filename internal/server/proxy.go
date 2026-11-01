package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// proxyPTY forwards terminal connections and closes them when requests are canceled.
func proxyPTY(
	w http.ResponseWriter,
	r *http.Request,
	target *url.URL,
	protection *http.CrossOriginProtection,
) {
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
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.URL.Path = "/pty"
			// Origin was checked at the public boundary.
			r.Out.Header.Del("Origin")
		},
	}
	proxy.ServeHTTP(writer, r)
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
