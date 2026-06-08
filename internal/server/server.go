package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	webassets "github.com/kurtisvg/ahh/internal/web"
)

// NewHandler builds the HTTP routes for the local Ahh server.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", index)
	mux.HandleFunc("GET /healthz", healthz)
	mux.Handle("GET /static/", noStore(http.StripPrefix("/static/", http.FileServer(http.FS(webassets.Files)))))
	return mux
}

// Listen opens a TCP listener for the configured HTTP host and port.
func Listen(host, port string) (net.Listener, error) {
	addr := net.JoinHostPort(host, port)
	return net.Listen("tcp", addr)
}

// Serve runs the HTTP server until the context is canceled or serving fails.
func Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           NewHandler(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("http server shutdown error", "error", err)
		}
	}()

	slog.Info("listening", "addr", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// healthz reports whether the local server can answer requests.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// noStore prevents development static assets from being cached by browsers or proxies.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets change often during early UI work, and proxied views may cache aggressively.
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// index serves the browser shell at the root route.
func index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	indexHTML, err := webassets.Files.ReadFile("index.html")
	if err != nil {
		http.Error(w, "failed to read index", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}
